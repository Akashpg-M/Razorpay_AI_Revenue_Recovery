package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/executor"
	"revenue-recovery/backend/internal/optimizer"
	"revenue-recovery/backend/internal/policy"
)

type fakeRepository struct {
	scheduled         ScheduledAction
	authorization     Authorization
	executing         int
	completed         int
	suppressed        string
	reassessmentReady bool
}

func (f *fakeRepository) ScheduleDecision(context.Context, decisioning.Snapshot) (*ScheduledAction, error) {
	return &f.scheduled, nil
}
func (f *fakeRepository) ClaimDue(context.Context, string, time.Time, time.Duration) (*ScheduledAction, error) {
	copy := f.scheduled
	return &copy, nil
}
func (f *fakeRepository) LoadAuthorization(context.Context, domain.ID, domain.ID) (Authorization, error) {
	return f.authorization, nil
}

func TestWorkerExecutesFreshHumanApprovedEscalation(t *testing.T) {
	now := time.Now().UTC()
	ctx := viableContext(now)
	ctx.MerchantContext.HighValueThresholdMinor = 100
	ctx.Case.AmountAtRiskMinor = 100
	repository := repositoryFor(domain.ActionSendReminder)
	repository.authorization.HumanApproved = true
	emails := &emailStore{}
	worker := NewWorker(repository, fakeContexts{ctx}, executor.NewRegistry(executor.NewEmailExecutor(emails)), "worker-1")
	if result, err := worker.RunOnce(context.Background()); err != nil || result == nil || emails.calls != 1 {
		t.Fatalf("human-approved action was not executed: result=%+v err=%v repo=%+v", result, err, repository)
	}
}
func (f *fakeRepository) MarkExecuting(context.Context, ScheduledAction, time.Time) error {
	f.executing++
	return nil
}
func (f *fakeRepository) CompleteExecution(context.Context, ScheduledAction, executor.Result, time.Time) error {
	f.completed++
	return nil
}
func (f *fakeRepository) MarkSuppressed(_ context.Context, _ ScheduledAction, reason string, _ time.Time) error {
	f.suppressed = reason
	return nil
}
func (f *fakeRepository) ClaimDueObservation(context.Context, string, time.Time, time.Duration) (*ScheduledAction, error) {
	copy := f.scheduled
	return &copy, nil
}
func (f *fakeRepository) PrepareReassessment(context.Context, ScheduledAction, time.Time) (bool, error) {
	return f.reassessmentReady, nil
}

type fakeContexts struct {
	value recoverycontext.RecoveryDecisionContext
}

func (f fakeContexts) Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error) {
	return f.value, nil
}

type emailStore struct{ calls int }

func (s *emailStore) CaptureEmail(context.Context, executor.Request, string, json.RawMessage) (string, bool, error) {
	s.calls++
	return "mail", true, nil
}

type fakeReassessor struct{ calls int }

func (f *fakeReassessor) Reassess(context.Context, domain.ID) error { f.calls++; return nil }

func viableContext(now time.Time) recoverycontext.RecoveryDecisionContext {
	return recoverycontext.RecoveryDecisionContext{
		Case:            domain.RecoveryCase{ID: "case-1", CustomerID: "customer-1", CurrentState: domain.StateScheduled, Version: 3, AmountAtRiskMinor: 10000, Currency: "INR", RecoveryDeadline: now.Add(time.Hour)},
		Diagnosis:       recoverycontext.Diagnosis{Confidence: .9},
		CustomerProfile: recoverycontext.CustomerProfile{},
		MerchantContext: recoverycontext.MerchantContext{AllowedChannels: []string{"email"}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5, MaximumIncentiveMinor: 0, Timezone: "UTC"},
		PaymentState:    recoverycontext.PaymentState{AvailableChannels: []string{"email"}},
	}
}

func repositoryFor(action domain.ActionType) *fakeRepository {
	return &fakeRepository{
		scheduled:     ScheduledAction{ID: "scheduled-1", CaseID: "case-1", DecisionID: "decision-1", RecoveryActionID: "action-1", Action: action, Status: "CLAIMED", AttemptCount: 1, MaxAttempts: 3, IdempotencyKey: "key", CaseVersionAtSchedule: 3},
		authorization: Authorization{DecisionCaseVersion: 3, Candidate: optimizer.Candidate{Action: action, NERVMinor: 100}, Gate: economicgate.Result{Decision: "ALLOW"}},
	}
}

func TestWorkerExecutesApprovedFreshAction(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repository := repositoryFor(domain.ActionSendReminder)
	emails := &emailStore{}
	worker := NewWorker(repository, fakeContexts{viableContext(now)}, executor.NewRegistry(executor.NewEmailExecutor(emails)), "worker-1")
	worker.now = func() time.Time { return now }
	result, err := worker.RunOnce(context.Background())
	if err != nil || result == nil || result.Status != "SUCCEEDED" || emails.calls != 1 || repository.executing != 1 || repository.completed != 1 {
		t.Fatalf("unexpected workflow: result=%+v err=%v repo=%+v", result, err, repository)
	}
}

func TestWorkerSuppressesRecoveredOrStaleActionBeforeSideEffect(t *testing.T) {
	now := time.Now().UTC()
	ctx := viableContext(now)
	ctx.Case.CurrentState = domain.StateRecovered
	repository := repositoryFor(domain.ActionSendReminder)
	emails := &emailStore{}
	worker := NewWorker(repository, fakeContexts{ctx}, executor.NewRegistry(executor.NewEmailExecutor(emails)), "worker-1")
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.suppressed != "STALE_OR_RECOVERED" || emails.calls != 0 {
		t.Fatalf("side effect was not suppressed: %+v", repository)
	}
}

func TestWorkerRechecksCurrentOptOutPolicy(t *testing.T) {
	now := time.Now().UTC()
	ctx := viableContext(now)
	ctx.CustomerProfile.OptedOut = true
	repository := repositoryFor(domain.ActionSendReminder)
	emails := &emailStore{}
	worker := NewWorker(repository, fakeContexts{ctx}, executor.NewRegistry(executor.NewEmailExecutor(emails)), "worker-1")
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.suppressed != "POLICY_RECHECK_DENY" || emails.calls != 0 {
		t.Fatalf("fresh policy was not enforced: %+v", repository)
	}
}

func TestWorkerDoesNotExecuteEscalatedOrActivePromiseCase(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		mutate func(*recoverycontext.RecoveryDecisionContext)
		want   string
	}{
		{"high value", func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.HighValueThresholdMinor = 100
			c.Case.AmountAtRiskMinor = 100
		}, "POLICY_RECHECK_ESCALATE"},
		{"active promise", func(c *recoverycontext.RecoveryDecisionContext) {
			c.ActivePromise = &domain.PromiseToPay{Status: "ACTIVE"}
		}, "POLICY_RECHECK_DENY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := viableContext(now)
			tc.mutate(&ctx)
			repository := repositoryFor(domain.ActionSendReminder)
			emails := &emailStore{}
			worker := NewWorker(repository, fakeContexts{ctx}, executor.NewRegistry(executor.NewEmailExecutor(emails)), "worker-1")
			if _, err := worker.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if repository.suppressed != tc.want || emails.calls != 0 {
				t.Fatalf("unexpected execution: %+v calls=%d", repository, emails.calls)
			}
		})
	}
}

func TestPermanentExecutorFailureTriggersReassessment(t *testing.T) {
	now := time.Now().UTC()
	ctx := viableContext(now)
	repository := repositoryFor(domain.ActionRetryNow)
	worker := NewWorker(repository, fakeContexts{ctx}, executor.NewRegistry(executor.NewRetryExecutor(nil)), "worker-1")
	reassessor := &fakeReassessor{}
	worker.SetReassessor(reassessor)
	result, err := worker.RunOnce(context.Background())
	if err == nil || result == nil || result.FailureClass != "PROVIDER_CAPABILITY_UNAVAILABLE" || reassessor.calls != 1 {
		t.Fatalf("expected permanent failure and reassessment: result=%+v err=%v calls=%d", result, err, reassessor.calls)
	}
}

func TestSchedulerDoesNotScheduleDeniedOrWaitDecisions(t *testing.T) {
	repository := repositoryFor(domain.ActionSendReminder)
	scheduler := NewScheduler(repository)
	if scheduled, err := scheduler.Schedule(context.Background(), decisioning.Snapshot{}); err != nil || scheduled != nil {
		t.Fatalf("denied decision scheduled: %+v %v", scheduled, err)
	}
	snapshot := decisioning.Snapshot{Policy: structPolicyApprove(), Gate: economicgate.Result{Decision: "ALLOW"}}
	snapshot.Decision.Optimization.Selected.Action = domain.ActionWait
	if scheduled, err := scheduler.Schedule(context.Background(), snapshot); err != nil || scheduled != nil {
		t.Fatalf("WAIT scheduled: %+v %v", scheduled, err)
	}
}

func TestObservationTimeoutStartsFreshDecisionCycle(t *testing.T) {
	repository := repositoryFor(domain.ActionSendReminder)
	repository.reassessmentReady = true
	worker := NewWorker(repository, fakeContexts{}, executor.NewRegistry(), "worker-1")
	reassessor := &fakeReassessor{}
	worker.SetReassessor(reassessor)
	if err := worker.RunObservationOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reassessor.calls != 1 {
		t.Fatalf("expected reassessment, got %d", reassessor.calls)
	}
}

func structPolicyApprove() policy.Result { return policy.Result{Decision: "APPROVE"} }

type coordinatorEvaluator struct {
	steps    *[]string
	snapshot decisioning.Snapshot
}

func (e coordinatorEvaluator) Evaluate(context.Context, domain.ID) (decisioning.Snapshot, error) {
	*e.steps = append(*e.steps, "evaluate")
	return e.snapshot, nil
}

type coordinatorRepository struct {
	steps      *[]string
	prepareErr error
	scheduled  *ScheduledAction
}

func (r coordinatorRepository) PrepareForDecision(context.Context, domain.ID, time.Time) error {
	*r.steps = append(*r.steps, "prepare")
	return r.prepareErr
}
func (r coordinatorRepository) SaveDecisionAndSchedule(_ context.Context, _ decisioning.Snapshot) (*ScheduledAction, error) {
	*r.steps = append(*r.steps, "atomic-save-and-schedule")
	return r.scheduled, nil
}

func TestDecisionCoordinatorPreparesBeforeAtomicDecisionAndSchedule(t *testing.T) {
	steps := []string{}
	snapshot := decisioning.Snapshot{Decision: decisioning.Decision{CaseID: "case-1", CaseVersion: 3}}
	scheduled := &ScheduledAction{ID: "scheduled-1", CaseID: "case-1"}
	coordinator := NewDecisionCoordinator(coordinatorEvaluator{steps: &steps, snapshot: snapshot}, coordinatorRepository{steps: &steps, scheduled: scheduled})
	gotSnapshot, gotScheduled, err := coordinator.Decide(context.Background(), "case-1")
	if err != nil || gotSnapshot.Decision.CaseVersion != 3 || gotScheduled.ID != "scheduled-1" {
		t.Fatalf("snapshot=%+v scheduled=%+v err=%v", gotSnapshot, gotScheduled, err)
	}
	want := []string{"prepare", "evaluate", "atomic-save-and-schedule"}
	if string(mustJSON(steps)) != string(mustJSON(want)) {
		t.Fatalf("steps=%v want=%v", steps, want)
	}
}

func TestDecisionCoordinatorRejectsStaleLifecycleBeforePersisting(t *testing.T) {
	steps := []string{}
	coordinator := NewDecisionCoordinator(coordinatorEvaluator{steps: &steps}, coordinatorRepository{steps: &steps, prepareErr: errors.New("stale")})
	if _, _, err := coordinator.Decide(context.Background(), "case-1"); err == nil {
		t.Fatal("expected stale lifecycle failure")
	}
	if len(steps) != 1 || steps[0] != "prepare" {
		t.Fatalf("decision work continued after stale prepare: %v", steps)
	}
}

func mustJSON(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}
