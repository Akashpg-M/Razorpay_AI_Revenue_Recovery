package attribution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"revenue-recovery/backend/internal/domain"
)

const RuleVersion = "attribution-v2"

// Precedence is part of the versioned attribution contract. Exact provider
// evidence wins over temporal inference; ambiguous overlaps remain visible in
// the evidence payload rather than being silently re-ordered.
var Precedence = []string{"EXACT_PROVIDER_REFERENCE", "PTP", "RETRY", "DIRECT_ACTION", "NATURAL", "UNKNOWN"}

type EvidenceCandidate struct {
	Name     string
	Category Category
}

func ResolveCandidates(candidates ...EvidenceCandidate) Category {
	for _, name := range Precedence {
		for _, candidate := range candidates {
			if candidate.Name == name {
				return candidate.Category
			}
		}
	}
	return Unknown
}

type Category string

const (
	DirectAction Category = "DIRECT_ACTION_ATTRIBUTED"
	Retry        Category = "RETRY_ATTRIBUTED"
	Promise      Category = "PTP_ATTRIBUTED"
	Natural      Category = "NATURAL_RECOVERY"
	Unknown      Category = "UNKNOWN"
)

type Record struct {
	ID                   domain.ID       `json:"attribution_id"`
	CaseID               domain.ID       `json:"case_id"`
	RecoveredAmountMinor int64           `json:"recovered_amount_minor"`
	PaymentReference     string          `json:"payment_reference"`
	Category             Category        `json:"category"`
	DecisionID           *domain.ID      `json:"decision_id,omitempty"`
	ActionID             *domain.ID      `json:"action_id,omitempty"`
	ExecutionID          *domain.ID      `json:"execution_id,omitempty"`
	PromiseID            *domain.ID      `json:"promise_id,omitempty"`
	Evidence             json.RawMessage `json:"evidence"`
	EvidenceStrength     string          `json:"evidence_strength"`
	RuleVersion          string          `json:"rule_version"`
	ObservedAt           time.Time       `json:"observed_at"`
	CreatedAt            time.Time       `json:"created_at"`
}
type ObserveInput struct {
	CaseID               domain.ID `json:"-"`
	RecoveredAmountMinor int64     `json:"recovered_amount_minor"`
	PaymentReference     string    `json:"payment_reference"`
	ObservedAt           time.Time `json:"observed_at"`
	CorrelationID        string    `json:"correlation_id"`
}
type Store interface {
	AttributeRecovery(context.Context, ObserveInput) (Record, bool, error)
	ListAttributions(context.Context, domain.ID) ([]Record, error)
}
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Observe(ctx context.Context, input ObserveInput) (Record, bool, error) {
	if input.CaseID == "" || input.PaymentReference == "" || input.RecoveredAmountMinor <= 0 {
		return Record{}, false, errors.New("case_id, payment_reference and a positive recovered amount are required")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = s.now().UTC()
	}
	if input.CorrelationID == "" {
		input.CorrelationID = "payment:" + input.PaymentReference
	}
	return s.store.AttributeRecovery(ctx, input)
}
func (s *Service) List(ctx context.Context, caseID domain.ID) ([]Record, error) {
	return s.store.ListAttributions(ctx, caseID)
}
