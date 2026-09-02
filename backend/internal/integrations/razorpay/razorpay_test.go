package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/domain"
)

func signature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"event":"payment.failed"}`)
	verifier := NewVerifier("secret")
	if err := verifier.Verify(body, signature("secret", body)); err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(body, signature("wrong", body)); err != ErrInvalidSignature {
		t.Fatalf("got %v", err)
	}
	modified := append([]byte(nil), body...)
	modified[len(modified)-1] = ' '
	if err := verifier.Verify(modified, signature("secret", body)); err != ErrInvalidSignature {
		t.Fatalf("modified payload got %v", err)
	}
}

type webhookStore struct {
	records   map[string]WebhookRecord
	processed int
}

func (s *webhookStore) InsertWebhook(_ context.Context, w WebhookRecord) (bool, error) {
	if _, ok := s.records[w.ProviderEventID]; ok {
		return false, nil
	}
	s.records[w.ProviderEventID] = w
	return true, nil
}
func (s *webhookStore) MarkWebhookProcessed(_ context.Context, id, status string, at time.Time) error {
	s.processed++
	w := s.records[id]
	w.ProcessingStatus = status
	w.ProcessedAt = &at
	s.records[id] = w
	return nil
}

type detectorStub struct{ calls int }

func (d *detectorStub) Detect(context.Context, detection.Adapter, json.RawMessage, string) (detection.Result, error) {
	d.calls++
	return detection.Result{Created: true, RiskDetected: true}, nil
}

func TestWebhookIngestionIsIdempotent(t *testing.T) {
	body := []byte(`{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_1","amount":10000,"currency":"INR","error_code":"insufficient_funds","created_at":1788177600,"notes":{"merchant_id":"m1","customer_id":"c1"}}},"subscription":{"entity":{"id":"sub_1"}}}}`)
	store := &webhookStore{records: map[string]WebhookRecord{}}
	detector := &detectorStub{}
	ingestor := NewIngestor("secret", store, detector)
	if _, duplicate, err := ingestor.Ingest(context.Background(), body, signature("secret", body), "evt_1"); err != nil || duplicate {
		t.Fatalf("first duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err := ingestor.Ingest(context.Background(), body, signature("secret", body), "evt_1"); err != nil || !duplicate {
		t.Fatalf("second duplicate=%v err=%v", duplicate, err)
	}
	if detector.calls != 1 {
		t.Fatalf("detector called %d times", detector.calls)
	}
}

func TestInvalidSignatureDoesNotPoisonEventID(t *testing.T) {
	body := []byte(`{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_1","amount":100,"currency":"INR","notes":{"merchant_id":"m1","customer_id":"c1"}}}}}`)
	store := &webhookStore{records: map[string]WebhookRecord{}}
	detector := &detectorStub{}
	ingestor := NewIngestor("secret", store, detector)
	_, _, err := ingestor.Ingest(context.Background(), body, "bad", "evt_bad")
	if err != ErrInvalidSignature {
		t.Fatalf("got %v", err)
	}
	if len(store.records) != 0 {
		t.Fatal("unauthenticated event must not reserve the provider event ID")
	}
	if _, duplicate, err := ingestor.Ingest(context.Background(), body, signature("secret", body), "evt_bad"); err != nil || duplicate {
		t.Fatalf("genuine redelivery duplicate=%v err=%v", duplicate, err)
	}
	if detector.calls != 1 {
		t.Fatalf("detector calls=%d", detector.calls)
	}
}

type resolverStub struct {
	caseID      domain.ID
	linkID      string
	referenceID string
}

func (r *resolverStub) ResolvePaymentLinkCase(_ context.Context, linkID, referenceID string, _ domain.ID, _ domain.ID, _ domain.ID) (domain.ID, error) {
	r.linkID, r.referenceID = linkID, referenceID
	return r.caseID, nil
}

type observerStub struct{ input attribution.ObserveInput }

func (o *observerStub) Observe(_ context.Context, input attribution.ObserveInput) (attribution.Record, bool, error) {
	o.input = input
	return attribution.Record{ID: "attr_1", CaseID: input.CaseID, PaymentReference: input.PaymentReference}, true, nil
}

func TestPaymentLinkPaidAttributesRecovery(t *testing.T) {
	body := []byte(`{"event":"payment_link.paid","created_at":1788177600,"payload":{"payment_link":{"entity":{"id":"plink_1","amount_paid":12500,"currency":"INR","status":"paid","reference_id":"scheduled_1","notes":{"merchant_id":"m1","customer_id":"c1","recovery_case_id":"case_1"}}},"payment":{"entity":{"id":"pay_1","amount":12500,"currency":"INR"}}}}`)
	store := &webhookStore{records: map[string]WebhookRecord{}}
	resolver := &resolverStub{caseID: "case_1"}
	observer := &observerStub{}
	ingestor := NewIngestor("secret", store, &detectorStub{})
	ingestor.SetRecoveryObserver(observer, resolver)
	result, duplicate, err := ingestor.Ingest(context.Background(), body, signature("secret", body), "evt_paid_1")
	if err != nil || duplicate {
		t.Fatalf("duplicate=%v err=%v", duplicate, err)
	}
	if result.Outcome != "RECOVERED" || !result.AttributionCreated || result.Attribution == nil {
		t.Fatalf("result=%+v", result)
	}
	if resolver.linkID != "plink_1" || resolver.referenceID != "scheduled_1" {
		t.Fatalf("resolver link=%q reference=%q", resolver.linkID, resolver.referenceID)
	}
	if observer.input.CaseID != "case_1" || observer.input.PaymentReference != "plink_1" || observer.input.RecoveredAmountMinor != 12500 || observer.input.CorrelationID != "evt_paid_1" {
		t.Fatalf("observe input=%+v", observer.input)
	}
	if store.records["evt_paid_1"].ProcessingStatus != "PROCESSED" {
		t.Fatalf("webhook=%+v", store.records["evt_paid_1"])
	}
}

func TestMalformedWebhookRejected(t *testing.T) {
	if _, _, _, err := NormalizeWebhook([]byte(`not-json`)); err == nil {
		t.Fatal("expected malformed webhook rejection")
	}
}

type linkStore struct {
	link   PaymentLink
	exists bool
	saves  int
}

func (s *linkStore) GetPaymentLink(context.Context, string) (PaymentLink, bool, error) {
	return s.link, s.exists, nil
}
func (s *linkStore) SavePaymentLink(_ context.Context, _ string, l PaymentLink, _ json.RawMessage) error {
	s.link = l
	s.exists = true
	s.saves++
	return nil
}

func TestPaymentLinkIdempotencyAndStatusLookup(t *testing.T) {
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/payment_links" {
			creates++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"plink_1","short_url":"https://rzp.io/i/x","status":"created","reference_id":"case_1"}`))
			return
		}
		if r.URL.Path == "/v1/payments/pay_1" {
			w.Write([]byte(`{"id":"pay_1","status":"captured","amount":10000,"currency":"INR"}`))
			return
		}
		if r.URL.Path == "/v1/payment_links/plink_1" {
			w.Write([]byte(`{"id":"plink_1","status":"created","reference_id":"case_1"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := NewClient(server.URL, "key", "secret")
	store := &linkStore{}
	executor := NewPaymentLinkExecutor(client, store)
	for range 2 {
		link, err := executor.Execute(context.Background(), "action_1", PaymentLinkRequest{Amount: 10000, Currency: "INR", ReferenceID: "case_1"})
		if err != nil || link.ID != "plink_1" {
			t.Fatalf("link=%+v err=%v", link, err)
		}
	}
	if creates != 1 || store.saves != 1 {
		t.Fatalf("creates=%d saves=%d", creates, store.saves)
	}
	payment, err := client.FetchPayment(context.Background(), "pay_1")
	if err != nil || payment.Status != "captured" {
		t.Fatalf("payment=%+v err=%v", payment, err)
	}
	fetched, err := executor.Reconcile(context.Background(), "action_1")
	if err != nil || fetched.ID != "plink_1" {
		t.Fatalf("fetched=%+v err=%v", fetched, err)
	}
}

func TestBaseURLNormalizationAndAuthenticationStatus(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RequestURI()
		username, password, ok := r.BasicAuth()
		if !ok || username != "rzp_test_example" || password != "secret" {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count":0,"items":[]}`))
	}))
	defer server.Close()
	client := NewClient(server.URL+"/v1/", "rzp_test_example", "secret")
	status := client.CheckAuthentication(context.Background())
	if !status.Configured || !status.Reachable || !status.Authenticated || status.Mode != "test" || status.HTTPStatus != 200 {
		t.Fatalf("status=%+v", status)
	}
	if path != "/v1/payments?count=1" {
		t.Fatalf("unexpected URL %q", path)
	}
}

func TestCredentialsAndLiveModeFailClosedWithoutHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls++ }))
	defer server.Close()
	missing := NewClient(server.URL, "", "").CheckAuthentication(context.Background())
	if missing.Configured || missing.ErrorCode != "credentials_not_configured" {
		t.Fatalf("missing=%+v", missing)
	}
	live := NewClient(server.URL, "rzp_live_forbidden", "secret").CheckAuthentication(context.Background())
	if live.ErrorCode != "live_mode_disabled" || calls != 0 {
		t.Fatalf("live=%+v calls=%d", live, calls)
	}
}

func TestAPIErrorDoesNotExposeProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"sensitive fixture"}}`))
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "key", "secret").FetchPayment(context.Background(), "pay_1")
	if err == nil || strings.Contains(err.Error(), "sensitive fixture") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestProviderErrorsAreTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "key", "secret").CreatePaymentLink(context.Background(), PaymentLinkRequest{Amount: 1, Currency: "INR"})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != 429 {
		t.Fatalf("got %#v", err)
	}
}
