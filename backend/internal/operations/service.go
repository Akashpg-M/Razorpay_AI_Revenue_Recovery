package operations

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/optimizer"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/policy"
)

type ReviewDecision string

const (
	Approve ReviewDecision = "APPROVE"
	Reject  ReviewDecision = "REJECT"
	Defer   ReviewDecision = "DEFER"
	Stop    ReviewDecision = "STOP"
)

type QueueItem struct {
	CaseID                     domain.ID           `json:"case_id"`
	CaseVersion                int64               `json:"case_version"`
	State                      domain.CaseState    `json:"state"`
	Priority                   string              `json:"priority"`
	Category                   string              `json:"category"`
	AmountAtRiskMinor          int64               `json:"amount_at_risk_minor"`
	Currency                   string              `json:"currency"`
	MerchantID                 domain.ID           `json:"merchant_id"`
	MerchantName               string              `json:"merchant_name"`
	MerchantPolicyVersion      int                 `json:"merchant_policy_version"`
	CustomerSafeReference      string              `json:"customer_safe_reference"`
	LeakType                   domain.LeakType     `json:"leak_type"`
	DecisionID                 domain.ID           `json:"decision_id"`
	PolicyEvaluationID         domain.ID           `json:"policy_evaluation_id"`
	EconomicGateID             domain.ID           `json:"economic_gate_id"`
	RecommendedAction          domain.ActionType   `json:"recommended_action"`
	ExpectedNERVMinor          int64               `json:"expected_nerv_minor"`
	ActionRecoveryProbability  float64             `json:"action_recovery_probability"`
	NaturalRecoveryProbability float64             `json:"natural_recovery_probability"`
	IncrementalUplift          float64             `json:"incremental_uplift"`
	DiagnosisConfidence        float64             `json:"diagnosis_confidence"`
	EscalationReasons          []string            `json:"escalation_reasons"`
	MerchantObjective          string              `json:"merchant_objective"`
	RecoveryDeadline           time.Time           `json:"recovery_deadline"`
	EscalatedAt                time.Time           `json:"escalated_at"`
	EscalationAgeSeconds       int64               `json:"escalation_age_seconds"`
	ReviewStatus               string              `json:"review_status"`
	ReviewAfter                *time.Time          `json:"review_after,omitempty"`
	Candidate                  optimizer.Candidate `json:"-"`
	Gate                       economicgate.Result `json:"-"`
}

type Filter struct {
	Category, MerchantID, Status, Priority, LeakType string
	MinAmountMinor, MaxAmountMinor                   int64
	DeadlineBefore                                   *time.Time
	Sort                                             string
}

type ReviewInput struct {
	Decision            ReviewDecision  `json:"decision"`
	OperatorID          string          `json:"operator_id"`
	ActorType           string          `json:"actor_type"`
	ActorMetadata       json.RawMessage `json:"actor_metadata"`
	ReasonCode          string          `json:"reason_code"`
	Notes               string          `json:"notes"`
	ExpectedCaseVersion int64           `json:"expected_case_version"`
	ReviewAfter         *time.Time      `json:"review_after"`
	IdempotencyKey      string          `json:"idempotency_key"`
}

type Review struct {
	ID                            string            `json:"approval_id"`
	CaseID                        domain.ID         `json:"case_id"`
	DecisionID                    domain.ID         `json:"decision_id"`
	PolicyEvaluationID            domain.ID         `json:"policy_evaluation_id"`
	RecommendedAction             domain.ActionType `json:"recommended_action"`
	OperatorID                    string            `json:"operator_id"`
	ActorType                     string            `json:"actor_type"`
	ActorMetadata                 json.RawMessage   `json:"actor_metadata"`
	Decision                      ReviewDecision    `json:"decision"`
	ReasonCode                    string            `json:"reason_code"`
	Notes                         string            `json:"notes"`
	CaseVersionAtReview           int64             `json:"case_version_at_review"`
	MerchantPolicyVersionAtReview int               `json:"merchant_policy_version_at_review"`
	ReviewAfter                   *time.Time        `json:"review_after,omitempty"`
	IdempotencyKey                string            `json:"idempotency_key"`
	ReauthorizationResult         string            `json:"reauthorization_result"`
	ReauthorizationReasonCodes    []string          `json:"reauthorization_reason_codes"`
	ScheduledActionID             *domain.ID        `json:"scheduled_action_id,omitempty"`
	CreatedAt                     time.Time         `json:"created_at"`
}

type Metrics struct {
	PendingReviews           int64   `json:"pending_reviews"`
	ValueAwaitingReviewMinor int64   `json:"value_awaiting_review_minor"`
	Approvals                int64   `json:"approvals"`
	Rejections               int64   `json:"rejections"`
	Deferrals                int64   `json:"deferrals"`
	Stops                    int64   `json:"stops"`
	StaleApprovals           int64   `json:"stale_approvals"`
	ExpiredReviews           int64   `json:"expired_reviews"`
	MedianReviewSeconds      float64 `json:"median_review_seconds"`
}

type ApplyCommand struct {
	Item        QueueItem
	Input       ReviewInput
	FreshPolicy policy.Result
	Now         time.Time
}
type Store interface {
	ListOperationsQueue(context.Context) ([]QueueItem, error)
	GetOperationsQueueItem(context.Context, domain.ID) (QueueItem, []Review, error)
	ApplyHumanReview(context.Context, ApplyCommand) (Review, *orchestrator.ScheduledAction, bool, error)
	OperationsMetrics(context.Context) (Metrics, error)
}
type ContextProvider interface {
	Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error)
}
type Service struct {
	store    Store
	contexts ContextProvider
	now      func() time.Time
}

func NewService(store Store, contexts ContextProvider) *Service {
	return &Service{store: store, contexts: contexts, now: time.Now}
}

func (s *Service) List(ctx context.Context, filter Filter) ([]QueueItem, error) {
	items, err := s.store.ListOperationsQueue(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := items[:0]
	for _, item := range items {
		item.EscalationAgeSeconds = max(0, int64(now.Sub(item.EscalatedAt).Seconds()))
		if filter.Category != "" && item.Category != filter.Category {
			continue
		}
		if filter.MerchantID != "" && string(item.MerchantID) != filter.MerchantID {
			continue
		}
		if filter.Status != "" && item.ReviewStatus != filter.Status {
			continue
		}
		if filter.Priority != "" && item.Priority != filter.Priority {
			continue
		}
		if filter.LeakType != "" && string(item.LeakType) != filter.LeakType {
			continue
		}
		if filter.MinAmountMinor > 0 && item.AmountAtRiskMinor < filter.MinAmountMinor {
			continue
		}
		if filter.MaxAmountMinor > 0 && item.AmountAtRiskMinor > filter.MaxAmountMinor {
			continue
		}
		if filter.DeadlineBefore != nil && item.RecoveryDeadline.After(*filter.DeadlineBefore) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if filter.Sort == "amount_asc" {
			if result[i].AmountAtRiskMinor != result[j].AmountAtRiskMinor {
				return result[i].AmountAtRiskMinor < result[j].AmountAtRiskMinor
			}
		} else if filter.Sort == "deadline_desc" {
			if !result[i].RecoveryDeadline.Equal(result[j].RecoveryDeadline) {
				return result[i].RecoveryDeadline.After(result[j].RecoveryDeadline)
			}
		} else if !result[i].RecoveryDeadline.Equal(result[j].RecoveryDeadline) {
			return result[i].RecoveryDeadline.Before(result[j].RecoveryDeadline)
		}
		if result[i].ExpectedNERVMinor != result[j].ExpectedNERVMinor {
			return result[i].ExpectedNERVMinor > result[j].ExpectedNERVMinor
		}
		return result[i].CaseID < result[j].CaseID
	})
	return result, nil
}
func (s *Service) Get(ctx context.Context, caseID domain.ID) (QueueItem, []Review, error) {
	return s.store.GetOperationsQueueItem(ctx, caseID)
}
func (s *Service) Metrics(ctx context.Context) (Metrics, error) {
	return s.store.OperationsMetrics(ctx)
}
func (s *Service) Review(ctx context.Context, caseID domain.ID, input ReviewInput) (Review, *orchestrator.ScheduledAction, bool, error) {
	if input.Decision != Approve && input.Decision != Reject && input.Decision != Defer && input.Decision != Stop {
		return Review{}, nil, false, errors.New("decision must be APPROVE, REJECT, DEFER, or STOP")
	}
	if strings.TrimSpace(input.OperatorID) == "" || strings.TrimSpace(input.ReasonCode) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return Review{}, nil, false, errors.New("operator_id, reason_code, and idempotency_key are required")
	}
	if input.ActorType != "OPERATOR" && input.ActorType != "SUPERVISOR" && input.ActorType != "SYSTEM_TEST" {
		return Review{}, nil, false, errors.New("actor_type must be OPERATOR, SUPERVISOR, or SYSTEM_TEST")
	}
	if input.Decision == Defer && (input.ReviewAfter == nil || !input.ReviewAfter.After(s.now())) {
		return Review{}, nil, false, errors.New("DEFER requires a future review_after")
	}
	item, _, err := s.store.GetOperationsQueueItem(ctx, caseID)
	if err != nil {
		return Review{}, nil, false, err
	}
	fresh := policy.Result{Decision: "APPROVE", ReasonCodes: []string{"HUMAN_REVIEW_NOT_REQUIRED"}}
	if input.Decision == Approve {
		decisionContext, contextErr := s.contexts.Get(ctx, caseID)
		if contextErr != nil {
			return Review{}, nil, false, contextErr
		}
		fresh = policy.Evaluate(decisionContext, item.DecisionID, decisionContext.Case.Version, item.Candidate, item.Gate, s.now().UTC())
	}
	return s.store.ApplyHumanReview(ctx, ApplyCommand{Item: item, Input: input, FreshPolicy: fresh, Now: s.now().UTC()})
}

var ErrNotReviewable = errors.New("case is not reviewable")
var ErrStaleApproval = errors.New("approval is stale")

// AuthorizeApproval is the final, execution-adjacent guard for a human approval.
// A reviewer can authorize intent, but cannot waive a newer case/policy state or
// a terminal policy result.
func AuthorizeApproval(state domain.CaseState, currentVersion, expectedVersion int64, currentPolicyVersion, observedPolicyVersion int, deadline, now time.Time, fresh policy.Result) (string, []string, bool) {
	reasons := append([]string(nil), fresh.ReasonCodes...)
	if state != domain.StateEscalated || currentVersion != expectedVersion || currentPolicyVersion != observedPolicyVersion || !deadline.After(now) {
		return "STALE_APPROVAL", append(reasons, "AUTHORITATIVE_STATE_CHANGED"), false
	}
	switch fresh.Decision {
	case "APPROVE", "ESCALATE":
		return "APPROVED", reasons, true
	case "STOP":
		return "STOPPED", reasons, false
	default:
		return "DENIED", reasons, false
	}
}
