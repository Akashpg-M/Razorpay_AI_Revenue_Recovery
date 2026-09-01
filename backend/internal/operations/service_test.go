package operations

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/policy"
)

type reviewStore struct {
	item    QueueItem
	command ApplyCommand
}

func (s *reviewStore) ListOperationsQueue(context.Context) ([]QueueItem, error) {
	return []QueueItem{s.item}, nil
}
func (s *reviewStore) GetOperationsQueueItem(context.Context, domain.ID) (QueueItem, []Review, error) {
	return s.item, nil, nil
}
func (s *reviewStore) ApplyHumanReview(_ context.Context, c ApplyCommand) (Review, *orchestrator.ScheduledAction, bool, error) {
	s.command = c
	return Review{Decision: c.Input.Decision, ActorMetadata: json.RawMessage(`{}`)}, nil, true, nil
}
func (s *reviewStore) OperationsMetrics(context.Context) (Metrics, error) { return Metrics{}, nil }

type unusedContext struct{}

func (unusedContext) Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error) {
	panic("context must not be loaded for non-approval reviews")
}

func TestAuthorizeApprovalRequiresFreshAuthority(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	approved := policy.Result{Decision: "APPROVE", ReasonCodes: []string{"ALL_POLICY_CHECKS_PASSED"}}
	tests := []struct {
		name                    string
		state                   domain.CaseState
		version, expected       int64
		policyVersion, observed int
		deadline                time.Time
		fresh                   policy.Result
		result                  string
		schedule                bool
	}{
		{"fresh", domain.StateEscalated, 4, 4, 3, 3, now.Add(time.Hour), approved, "APPROVED", true},
		{"case changed", domain.StateEscalated, 5, 4, 3, 3, now.Add(time.Hour), approved, "STALE_APPROVAL", false},
		{"merchant policy changed", domain.StateEscalated, 4, 4, 4, 3, now.Add(time.Hour), approved, "STALE_APPROVAL", false},
		{"already recovered", domain.StateRecovered, 4, 4, 3, 3, now.Add(time.Hour), approved, "STALE_APPROVAL", false},
		{"deadline expired", domain.StateEscalated, 4, 4, 3, 3, now, approved, "STALE_APPROVAL", false},
		{"live policy denies", domain.StateEscalated, 4, 4, 3, 3, now.Add(time.Hour), policy.Result{Decision: "DENY", ReasonCodes: []string{"CUSTOMER_OPT_OUT"}}, "DENIED", false},
		{"live policy stops", domain.StateEscalated, 4, 4, 3, 3, now.Add(time.Hour), policy.Result{Decision: "STOP", ReasonCodes: []string{"PAYMENT_ALREADY_RECOVERED"}}, "STOPPED", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, schedule := AuthorizeApproval(tt.state, tt.version, tt.expected, tt.policyVersion, tt.observed, tt.deadline, now, tt.fresh)
			if result != tt.result || schedule != tt.schedule {
				t.Fatalf("got %s/%v, want %s/%v", result, schedule, tt.result, tt.schedule)
			}
		})
	}
}

func TestRejectAndDeferDoNotInvokePolicyOrSchedule(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, decision := range []ReviewDecision{Reject, Defer, Stop} {
		t.Run(string(decision), func(t *testing.T) {
			store := &reviewStore{item: QueueItem{CaseID: "case-1", CaseVersion: 4}}
			service := NewService(store, unusedContext{})
			service.now = func() time.Time { return now }
			input := ReviewInput{Decision: decision, OperatorID: "operator-1", ActorType: "OPERATOR", ReasonCode: "REVIEWED", IdempotencyKey: "key-" + string(decision), ExpectedCaseVersion: 4}
			if decision == Defer {
				future := now.Add(time.Hour)
				input.ReviewAfter = &future
			}
			_, scheduled, _, err := service.Review(context.Background(), "case-1", input)
			if err != nil {
				t.Fatal(err)
			}
			if scheduled != nil {
				t.Fatal("non-approval review scheduled an action")
			}
			if store.command.Input.Decision != decision {
				t.Fatal("review was not durably delegated")
			}
		})
	}
}
