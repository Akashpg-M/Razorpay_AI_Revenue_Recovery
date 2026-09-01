package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"revenue-recovery/backend/internal/domain"
)

type SubscriptionEvent struct {
	EventType           string          `json:"event_type"`
	MerchantID          domain.ID       `json:"merchant_id"`
	CustomerID          domain.ID       `json:"customer_id"`
	AmountMinor         int64           `json:"amount_minor"`
	Currency            string          `json:"currency"`
	PaymentID           string          `json:"payment_id"`
	SubscriptionID      string          `json:"subscription_id"`
	OrderID             string          `json:"order_id"`
	FailureCode         string          `json:"failure_code"`
	FailureCategory     string          `json:"failure_category"`
	OccurredAt          time.Time       `json:"occurred_at"`
	RecoveryWindowHours int             `json:"recovery_window_hours"`
	Metadata            json.RawMessage `json:"metadata"`
}

type SubscriptionAdapter struct{ Provider string }

var subscriptionEvents = map[string]struct{}{
	"payment.failed": {}, "subscription.pending": {}, "subscription.halted": {},
	"mandate.failed": {}, "payment.mandate.failed": {},
}

func (a SubscriptionAdapter) Normalize(_ context.Context, payload json.RawMessage, providerEventID string) (*NormalizedLeak, error) {
	var event SubscriptionEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("decode subscription event: %w", err)
	}
	if _, ok := subscriptionEvents[event.EventType]; !ok {
		return nil, fmt.Errorf("unsupported subscription event %q", event.EventType)
	}
	if event.SubscriptionID == "" && event.PaymentID == "" {
		return nil, fmt.Errorf("payment_id or subscription_id is required")
	}
	if event.RecoveryWindowHours <= 0 {
		event.RecoveryWindowHours = 7 * 24
	}
	category := event.FailureCategory
	if category == "" {
		category = normalizeFailureCode(event.FailureCode)
	}
	references, _ := json.Marshal(map[string]string{"payment_id": event.PaymentID, "subscription_id": event.SubscriptionID, "order_id": event.OrderID})
	source := event.SubscriptionID
	if source == "" {
		source = event.PaymentID
	}
	provider := a.Provider
	if provider == "" {
		provider = "normalized"
	}
	return &NormalizedLeak{Provider: provider, ProviderEventID: providerEventID, EventType: event.EventType,
		LeakType: domain.FailedSubscription, MerchantID: event.MerchantID, CustomerID: event.CustomerID,
		AmountMinor: event.AmountMinor, Currency: event.Currency, SourceReference: source, SourceStatus: "FAILED",
		FailureCategory: category, OccurredAt: event.OccurredAt, RecoveryDeadline: event.OccurredAt.Add(time.Duration(event.RecoveryWindowHours) * time.Hour),
		ProviderReferences: references, Context: event.Metadata}, nil
}

func normalizeFailureCode(code string) string {
	switch code {
	case "BAD_REQUEST_PAYMENT_CARD_INSUFFICIENT_BALANCE", "insufficient_funds":
		return "INSUFFICIENT_FUNDS"
	case "BAD_REQUEST_PAYMENT_CARD_EXPIRED", "invalid_payment_method":
		return "PAYMENT_METHOD_INVALID"
	case "mandate_failed", "mandate_revoked":
		return "MANDATE_FAILURE"
	case "hard_decline", "payment_not_allowed":
		return "HARD_DECLINE"
	case "gateway_timeout", "bank_unavailable", "server_error":
		return "TEMPORARY_BANK_FAILURE"
	default:
		return "CUSTOMER_INTENT_OR_UNKNOWN"
	}
}
