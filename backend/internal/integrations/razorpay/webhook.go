package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/domain"
)

var ErrInvalidSignature = errors.New("invalid Razorpay webhook signature")

type Verifier struct{ secret []byte }

func NewVerifier(secret string) *Verifier { return &Verifier{secret: []byte(secret)} }

func (v *Verifier) Verify(body []byte, signature string) error {
	if len(v.secret) == 0 {
		return errors.New("RAZORPAY_WEBHOOK_SECRET is not configured")
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), provided) {
		return ErrInvalidSignature
	}
	return nil
}

type WebhookRecord struct {
	ID                 string
	ProviderEventID    string
	EventType          string
	SignatureStatus    string
	ProcessingStatus   string
	Payload            json.RawMessage
	ProviderReferences json.RawMessage
	ReceivedAt         time.Time
	ProcessedAt        *time.Time
}

type WebhookStore interface {
	InsertWebhook(context.Context, WebhookRecord) (bool, error)
	MarkWebhookProcessed(context.Context, string, string, time.Time) error
}

type DetectionService interface {
	Detect(context.Context, detection.Adapter, json.RawMessage, string) (detection.Result, error)
}

type RecoveryObserver interface {
	Observe(context.Context, attribution.ObserveInput) (attribution.Record, bool, error)
}

type PaymentLinkCaseResolver interface {
	ResolvePaymentLinkCase(context.Context, string, string, domain.ID, domain.ID, domain.ID) (domain.ID, error)
}

type IngestResult struct {
	Case               *domain.RecoveryCase `json:"recovery_case,omitempty"`
	Created            bool                 `json:"created,omitempty"`
	RiskDetected       bool                 `json:"risk_detected,omitempty"`
	Outcome            string               `json:"outcome,omitempty"`
	Attribution        *attribution.Record  `json:"attribution,omitempty"`
	AttributionCreated bool                 `json:"attribution_created,omitempty"`
}

type Ingestor struct {
	verifier *Verifier
	store    WebhookStore
	detector DetectionService
	observer RecoveryObserver
	resolver PaymentLinkCaseResolver
	adapter  detection.SubscriptionAdapter
	now      func() time.Time
}

func NewIngestor(secret string, store WebhookStore, detector DetectionService) *Ingestor {
	return &Ingestor{verifier: NewVerifier(secret), store: store, detector: detector,
		adapter: detection.SubscriptionAdapter{Provider: "razorpay"}, now: time.Now}
}

func (i *Ingestor) SetRecoveryObserver(observer RecoveryObserver, resolver PaymentLinkCaseResolver) {
	i.observer = observer
	i.resolver = resolver
}

func (i *Ingestor) Ingest(ctx context.Context, body []byte, signature, eventID string) (IngestResult, bool, error) {
	if eventID == "" {
		return IngestResult{}, false, errors.New("X-Razorpay-Event-Id is required")
	}
	if err := i.verifier.Verify(body, signature); err != nil {
		// Never reserve an event ID for an unauthenticated request. Otherwise an
		// attacker (or a bad test) can make the later genuine delivery look like a
		// duplicate and permanently suppress it.
		return IngestResult{}, false, err
	}
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return IngestResult{}, false, fmt.Errorf("decode Razorpay webhook: %w", err)
	}
	if envelope.Event == "" {
		return IngestResult{}, false, errors.New("Razorpay webhook event is required")
	}
	references := webhookReferences(envelope)
	now := i.now().UTC()
	record := WebhookRecord{ID: eventID, ProviderEventID: eventID, EventType: envelope.Event, SignatureStatus: "VERIFIED",
		ProcessingStatus: "RECEIVED", Payload: json.RawMessage(body), ProviderReferences: references, ReceivedAt: now}
	created, err := i.store.InsertWebhook(ctx, record)
	if err != nil {
		return IngestResult{}, false, err
	}
	if !created {
		return IngestResult{}, true, nil
	}
	if envelope.Event == "payment_link.paid" {
		result, observeErr := i.observePaymentLinkPaid(ctx, envelope, eventID)
		return result, false, i.finish(ctx, eventID, observeErr)
	}
	event, _, _, err := NormalizeWebhook(body)
	if err != nil {
		return IngestResult{}, false, i.finish(ctx, eventID, err)
	}
	normalized, _ := json.Marshal(event)
	result, err := i.detector.Detect(ctx, i.adapter, normalized, eventID)
	return IngestResult{Case: &result.Case, Created: result.Created, RiskDetected: result.RiskDetected}, false, i.finish(ctx, eventID, err)
}

func (i *Ingestor) finish(ctx context.Context, eventID string, processingErr error) error {
	status := "PROCESSED"
	if processingErr != nil {
		status = "FAILED"
	}
	if err := i.store.MarkWebhookProcessed(ctx, eventID, status, i.now().UTC()); err != nil && processingErr == nil {
		return err
	}
	return processingErr
}

func (i *Ingestor) observePaymentLinkPaid(ctx context.Context, envelope webhookEnvelope, eventID string) (IngestResult, error) {
	if i.observer == nil || i.resolver == nil {
		return IngestResult{}, errors.New("Razorpay recovery attribution is not configured")
	}
	link := envelope.Payload.PaymentLink.Entity
	if link.ID == "" {
		return IngestResult{}, errors.New("payment_link.paid payload is missing payment_link.entity.id")
	}
	merchantID := domain.ID(noteString(link.Notes["merchant_id"]))
	customerID := domain.ID(noteString(link.Notes["customer_id"]))
	noteCaseID := domain.ID(noteString(link.Notes["recovery_case_id"]))
	caseID, err := i.resolver.ResolvePaymentLinkCase(ctx, link.ID, link.ReferenceID, noteCaseID, merchantID, customerID)
	if err != nil {
		return IngestResult{}, fmt.Errorf("resolve recovery case for Razorpay payment link: %w", err)
	}
	amount := envelope.Payload.Payment.Entity.Amount
	if amount <= 0 {
		amount = link.AmountPaid
	}
	observedAt := time.Unix(envelope.CreatedAt, 0).UTC()
	if envelope.CreatedAt == 0 {
		observedAt = i.now().UTC()
	}
	record, created, err := i.observer.Observe(ctx, attribution.ObserveInput{CaseID: caseID, RecoveredAmountMinor: amount,
		PaymentReference: link.ID, ObservedAt: observedAt, CorrelationID: eventID})
	if errors.Is(err, attribution.ErrCaseTerminal) {
		return IngestResult{Outcome: "IGNORED_TERMINAL_CASE"}, nil
	}
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Outcome: "RECOVERED", Attribution: &record, AttributionCreated: created}, nil
}

type webhookEnvelope struct {
	Event     string `json:"event"`
	CreatedAt int64  `json:"created_at"`
	Payload   struct {
		Payment struct {
			Entity providerPayment `json:"entity"`
		} `json:"payment"`
		Subscription struct {
			Entity providerSubscription `json:"entity"`
		} `json:"subscription"`
		PaymentLink struct {
			Entity providerPaymentLink `json:"entity"`
		} `json:"payment_link"`
	} `json:"payload"`
}
type providerPayment struct {
	ID        string         `json:"id"`
	Amount    int64          `json:"amount"`
	Currency  string         `json:"currency"`
	OrderID   string         `json:"order_id"`
	ErrorCode string         `json:"error_code"`
	CreatedAt int64          `json:"created_at"`
	Notes     map[string]any `json:"notes"`
}
type providerSubscription struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`
	Notes  map[string]any `json:"notes"`
}
type providerPaymentLink struct {
	ID          string         `json:"id"`
	Amount      int64          `json:"amount"`
	AmountPaid  int64          `json:"amount_paid"`
	Currency    string         `json:"currency"`
	Status      string         `json:"status"`
	ReferenceID string         `json:"reference_id"`
	Notes       map[string]any `json:"notes"`
}

func webhookReferences(envelope webhookEnvelope) json.RawMessage {
	p := envelope.Payload.Payment.Entity
	s := envelope.Payload.Subscription.Entity
	l := envelope.Payload.PaymentLink.Entity
	references, _ := json.Marshal(map[string]string{"payment_id": p.ID, "subscription_id": s.ID, "order_id": p.OrderID,
		"payment_link_id": l.ID, "payment_link_reference_id": l.ReferenceID})
	return references
}

func NormalizeWebhook(body []byte) (detection.SubscriptionEvent, string, json.RawMessage, error) {
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return detection.SubscriptionEvent{}, "", nil, fmt.Errorf("decode Razorpay webhook: %w", err)
	}
	if envelope.Event == "" {
		return detection.SubscriptionEvent{}, "", nil, errors.New("Razorpay webhook event is required")
	}
	p := envelope.Payload.Payment.Entity
	s := envelope.Payload.Subscription.Entity
	notes := p.Notes
	if len(notes) == 0 {
		notes = s.Notes
	}
	merchantID, _ := notes["merchant_id"].(string)
	customerID, _ := notes["customer_id"].(string)
	if merchantID == "" || customerID == "" {
		return detection.SubscriptionEvent{}, "", nil, errors.New("merchant_id and customer_id must be present in provider notes")
	}
	occurred := time.Unix(p.CreatedAt, 0).UTC()
	if p.CreatedAt == 0 {
		occurred = time.Now().UTC()
	}
	internalType := envelope.Event
	if envelope.Event == "subscription.charged" {
		return detection.SubscriptionEvent{}, envelope.Event, nil, errors.New("success webhook does not represent new revenue risk")
	}
	if p.Amount == 0 {
		p.Amount = noteInt64(notes["amount_minor"])
	}
	if p.Currency == "" {
		p.Currency, _ = notes["currency"].(string)
	}
	references := webhookReferences(envelope)
	return detection.SubscriptionEvent{EventType: internalType, MerchantID: domain.ID(merchantID), CustomerID: domain.ID(customerID),
		AmountMinor: p.Amount, Currency: p.Currency, PaymentID: p.ID, SubscriptionID: s.ID, OrderID: p.OrderID,
		FailureCode: p.ErrorCode, OccurredAt: occurred, RecoveryWindowHours: 168}, envelope.Event, references, nil
}

func noteString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func noteInt64(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}
