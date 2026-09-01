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

type Ingestor struct {
	verifier *Verifier
	store    WebhookStore
	detector DetectionService
	adapter  detection.SubscriptionAdapter
	now      func() time.Time
}

func NewIngestor(secret string, store WebhookStore, detector DetectionService) *Ingestor {
	return &Ingestor{verifier: NewVerifier(secret), store: store, detector: detector,
		adapter: detection.SubscriptionAdapter{Provider: "razorpay"}, now: time.Now}
}

func (i *Ingestor) Ingest(ctx context.Context, body []byte, signature, eventID string) (detection.Result, bool, error) {
	if eventID == "" {
		return detection.Result{}, false, errors.New("X-Razorpay-Event-Id is required")
	}
	if err := i.verifier.Verify(body, signature); err != nil {
		var envelope webhookEnvelope
		_ = json.Unmarshal(body, &envelope)
		now := i.now().UTC()
		_, _ = i.store.InsertWebhook(ctx, WebhookRecord{ID: eventID, ProviderEventID: eventID, EventType: envelope.Event,
			SignatureStatus: "INVALID", ProcessingStatus: "IGNORED", Payload: json.RawMessage(body), ProviderReferences: json.RawMessage(`{}`), ReceivedAt: now})
		return detection.Result{}, false, err
	}
	event, eventType, references, err := NormalizeWebhook(body)
	if err != nil {
		return detection.Result{}, false, err
	}
	now := i.now().UTC()
	normalized, _ := json.Marshal(event)
	record := WebhookRecord{ID: eventID, ProviderEventID: eventID, EventType: eventType, SignatureStatus: "VERIFIED",
		ProcessingStatus: "RECEIVED", Payload: json.RawMessage(body), ProviderReferences: references, ReceivedAt: now}
	created, err := i.store.InsertWebhook(ctx, record)
	if err != nil {
		return detection.Result{}, false, err
	}
	if !created {
		return detection.Result{}, true, nil
	}
	result, err := i.detector.Detect(ctx, i.adapter, normalized, eventID)
	status := "PROCESSED"
	if err != nil {
		status = "FAILED"
	}
	if markErr := i.store.MarkWebhookProcessed(ctx, eventID, status, i.now().UTC()); markErr != nil && err == nil {
		err = markErr
	}
	return result, false, err
}

type webhookEnvelope struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity providerPayment `json:"entity"`
		} `json:"payment"`
		Subscription struct {
			Entity providerSubscription `json:"entity"`
		} `json:"subscription"`
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
	references, _ := json.Marshal(map[string]string{"payment_id": p.ID, "subscription_id": s.ID, "order_id": p.OrderID})
	return detection.SubscriptionEvent{EventType: internalType, MerchantID: domain.ID(merchantID), CustomerID: domain.ID(customerID),
		AmountMinor: p.Amount, Currency: p.Currency, PaymentID: p.ID, SubscriptionID: s.ID, OrderID: p.OrderID,
		FailureCode: p.ErrorCode, OccurredAt: occurred, RecoveryWindowHours: 168}, envelope.Event, references, nil
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
