package detection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/recovery"
)

type NormalizedLeak struct {
	Provider           string          `json:"provider"`
	ProviderEventID    string          `json:"provider_event_id"`
	EventType          string          `json:"event_type"`
	LeakType           domain.LeakType `json:"leak_type"`
	MerchantID         domain.ID       `json:"merchant_id"`
	CustomerID         domain.ID       `json:"customer_id"`
	AmountMinor        int64           `json:"amount_minor"`
	Currency           string          `json:"currency"`
	SourceReference    string          `json:"source_reference"`
	SourceStatus       string          `json:"source_status"`
	FailureCategory    string          `json:"failure_category"`
	OccurredAt         time.Time       `json:"occurred_at"`
	RecoveryDeadline   time.Time       `json:"recovery_deadline"`
	ProviderReferences json.RawMessage `json:"provider_references"`
	Context            json.RawMessage `json:"context"`
}

func (n NormalizedLeak) Validate() error {
	if n.MerchantID == "" || n.CustomerID == "" || n.SourceReference == "" || n.ProviderEventID == "" {
		return errors.New("merchant_id, customer_id, source_reference and provider_event_id are required")
	}
	if n.AmountMinor <= 0 || len(n.Currency) != 3 {
		return errors.New("amount_minor must be positive and currency must contain 3 letters")
	}
	if n.LeakType != domain.FailedSubscription && n.LeakType != domain.CheckoutAbandonment {
		return fmt.Errorf("unsupported leak type %q", n.LeakType)
	}
	if n.OccurredAt.IsZero() || !n.RecoveryDeadline.After(n.OccurredAt) {
		return errors.New("valid occurred_at and later recovery_deadline are required")
	}
	return nil
}

type Adapter interface {
	Normalize(context.Context, json.RawMessage, string) (*NormalizedLeak, error)
}

type Result struct {
	Case         domain.RecoveryCase `json:"recovery_case"`
	Created      bool                `json:"created"`
	RiskDetected bool                `json:"risk_detected"`
}

type Service struct{ recovery *recovery.Service }

func NewService(service *recovery.Service) *Service { return &Service{recovery: service} }

func (s *Service) Detect(ctx context.Context, adapter Adapter, payload json.RawMessage, providerEventID string) (Result, error) {
	leak, err := adapter.Normalize(ctx, payload, providerEventID)
	if err != nil {
		return Result{}, err
	}
	if leak == nil {
		return Result{RiskDetected: false}, nil
	}
	if err := leak.Validate(); err != nil {
		return Result{}, err
	}
	existing, err := s.recovery.GetCaseBySource(ctx, leak.MerchantID, leak.SourceReference)
	if err == nil {
		return Result{Case: existing, Created: false, RiskDetected: true}, nil
	}
	if !errors.Is(err, recovery.ErrNotFound) {
		return Result{}, err
	}
	failureContext, _ := json.Marshal(map[string]any{
		"provider": leak.Provider, "provider_event_id": leak.ProviderEventID,
		"source_event_type": leak.EventType, "failure_category": leak.FailureCategory,
		"occurred_at": leak.OccurredAt, "provider_references": leak.ProviderReferences,
		"adapter_context": leak.Context,
	})
	c, err := s.recovery.CreateCase(ctx, recovery.CreateCaseInput{
		LeakType: leak.LeakType, MerchantID: leak.MerchantID, CustomerID: leak.CustomerID,
		AmountAtRiskMinor: leak.AmountMinor, Currency: strings.ToUpper(leak.Currency),
		SourceReference: leak.SourceReference, SourceStatus: leak.SourceStatus,
		FailureOrLeakContext: failureContext, RecoveryDeadline: leak.RecoveryDeadline,
		Actor: domain.Actor{Type: "SYSTEM", ID: "detection:" + leak.Provider}, CorrelationID: leak.ProviderEventID,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Case: c, Created: true, RiskDetected: true}, nil
}
