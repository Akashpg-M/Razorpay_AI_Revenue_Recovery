package detection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"revenue-recovery/backend/internal/domain"
)

type CheckoutEvent struct {
	EventType         string          `json:"event_type"`
	CheckoutID        string          `json:"checkout_id"`
	MerchantID        domain.ID       `json:"merchant_id"`
	CustomerID        domain.ID       `json:"customer_id"`
	AmountMinor       int64           `json:"amount_minor"`
	Currency          string          `json:"currency"`
	CheckoutStage     string          `json:"checkout_stage"`
	PaymentMethod     string          `json:"payment_method"`
	FailureCode       string          `json:"failure_code"`
	AbandonmentReason string          `json:"abandonment_reason"`
	OccurredAt        time.Time       `json:"occurred_at"`
	ValidUntil        time.Time       `json:"valid_until"`
	Metadata          json.RawMessage `json:"metadata"`
}

type CheckoutSession struct {
	CheckoutID    string
	MerchantID    domain.ID
	CustomerID    domain.ID
	AmountMinor   int64
	Currency      string
	Stage         string
	PaymentMethod string
	ValidUntil    time.Time
}

type CheckoutStateStore interface {
	UpsertCheckout(context.Context, CheckoutSession) (CheckoutSession, error)
	GetCheckout(context.Context, string) (CheckoutSession, error)
}

type CheckoutAdapter struct{ Store CheckoutStateStore }

func (a CheckoutAdapter) Normalize(ctx context.Context, payload json.RawMessage, providerEventID string) (*NormalizedLeak, error) {
	var event CheckoutEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("decode checkout event: %w", err)
	}
	if event.CheckoutID == "" || event.EventType == "" {
		return nil, errors.New("checkout_id and event_type are required")
	}
	if a.Store == nil {
		return nil, errors.New("checkout state store is required")
	}
	session, err := a.Store.GetCheckout(ctx, event.CheckoutID)
	if err != nil && event.EventType != "CHECKOUT_STARTED" {
		return nil, fmt.Errorf("checkout state not found: %w", err)
	}
	if event.EventType == "CHECKOUT_STARTED" {
		session = CheckoutSession{CheckoutID: event.CheckoutID, MerchantID: event.MerchantID, CustomerID: event.CustomerID,
			AmountMinor: event.AmountMinor, Currency: event.Currency, Stage: event.CheckoutStage, ValidUntil: event.ValidUntil}
		if session.ValidUntil.IsZero() {
			session.ValidUntil = event.OccurredAt.Add(24 * time.Hour)
		}
	} else {
		if event.CheckoutStage != "" {
			session.Stage = event.CheckoutStage
		}
		if event.PaymentMethod != "" {
			session.PaymentMethod = event.PaymentMethod
		}
	}
	if _, err = a.Store.UpsertCheckout(ctx, session); err != nil {
		return nil, err
	}
	switch event.EventType {
	case "CHECKOUT_STARTED", "CHECKOUT_PAYMENT_SELECTED":
		return nil, nil
	case "CHECKOUT_PAYMENT_FAILED", "CHECKOUT_ABANDONED":
	default:
		return nil, fmt.Errorf("unsupported checkout event %q", event.EventType)
	}
	if !session.ValidUntil.After(event.OccurredAt) {
		return nil, errors.New("checkout is expired and not recoverable")
	}
	category := "UNKNOWN_ABANDONMENT"
	if event.EventType == "CHECKOUT_PAYMENT_FAILED" {
		category = "PAYMENT_FAILURE"
		if event.FailureCode == "method_mismatch" {
			category = "PAYMENT_METHOD_MISMATCH"
		}
	} else {
		switch event.AbandonmentReason {
		case "payment_friction":
			category = "PAYMENT_FRICTION"
		case "method_mismatch":
			category = "PAYMENT_METHOD_MISMATCH"
		case "delayed_intent":
			category = "DELAYED_INTENT"
		case "price_hesitation":
			category = "PRICE_OR_VALUE_HESITATION"
		}
	}
	contextPayload, _ := json.Marshal(map[string]any{"checkout_stage": session.Stage, "payment_method": session.PaymentMethod, "failure_code": event.FailureCode, "metadata": event.Metadata})
	return &NormalizedLeak{Provider: "checkout-demo", ProviderEventID: providerEventID, EventType: event.EventType,
		LeakType: domain.CheckoutAbandonment, MerchantID: session.MerchantID, CustomerID: session.CustomerID,
		AmountMinor: session.AmountMinor, Currency: session.Currency, SourceReference: session.CheckoutID,
		SourceStatus: "ABANDONED", FailureCategory: category, OccurredAt: event.OccurredAt,
		RecoveryDeadline: session.ValidUntil, Context: contextPayload, ProviderReferences: json.RawMessage(`{}`)}, nil
}
