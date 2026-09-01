package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"revenue-recovery/backend/internal/detection"
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

func TestInvalidSignaturePersistedAndRejected(t *testing.T) {
	body := []byte(`{"event":"payment.failed"}`)
	store := &webhookStore{records: map[string]WebhookRecord{}}
	ingestor := NewIngestor("secret", store, &detectorStub{})
	_, _, err := ingestor.Ingest(context.Background(), body, "bad", "evt_bad")
	if err != ErrInvalidSignature {
		t.Fatalf("got %v", err)
	}
	if store.records["evt_bad"].SignatureStatus != "INVALID" {
		t.Fatal("invalid signature metadata not persisted")
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
