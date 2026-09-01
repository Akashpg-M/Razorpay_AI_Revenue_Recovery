package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/executor"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/optimizer"
	"revenue-recovery/backend/internal/policy"
)

type ScheduledAction struct {
	ID                    domain.ID         `json:"scheduled_action_id"`
	CaseID                domain.ID         `json:"case_id"`
	DecisionID            domain.ID         `json:"decision_id"`
	PolicyEvaluationID    domain.ID         `json:"policy_evaluation_id"`
	RecoveryActionID      domain.ID         `json:"recovery_action_id"`
	Action                domain.ActionType `json:"action"`
	Parameters            json.RawMessage   `json:"parameters"`
	ScheduledFor          time.Time         `json:"scheduled_for"`
	Status                string            `json:"status"`
	AttemptCount          int               `json:"attempt_count"`
	MaxAttempts           int               `json:"max_attempts"`
	IdempotencyKey        string            `json:"idempotency_key"`
	CaseVersionAtSchedule int64             `json:"case_version_at_schedule"`
	NextRetryAt           *time.Time        `json:"next_retry_at,omitempty"`
}

type WorkflowView struct {
	Case             domain.RecoveryCase `json:"case"`
	LatestDecision   json.RawMessage     `json:"latest_decision,omitempty"`
	LatestPolicy     json.RawMessage     `json:"latest_policy_evaluation,omitempty"`
	ScheduledActions []ScheduledAction   `json:"scheduled_actions"`
	Executions       []json.RawMessage   `json:"executions"`
}

type Authorization struct {
	DecisionCaseVersion int64
	Candidate           optimizer.Candidate
	Gate                economicgate.Result
}

type Repository interface {
	ScheduleDecision(context.Context, decisioning.Snapshot) (*ScheduledAction, error)
	ClaimDue(context.Context, string, time.Time, time.Duration) (*ScheduledAction, error)
	LoadAuthorization(context.Context, domain.ID) (Authorization, error)
	MarkExecuting(context.Context, ScheduledAction, time.Time) error
	CompleteExecution(context.Context, ScheduledAction, executor.Result, time.Time) error
	MarkSuppressed(context.Context, ScheduledAction, string, time.Time) error
	ClaimDueObservation(context.Context, string, time.Time, time.Duration) (*ScheduledAction, error)
	PrepareReassessment(context.Context, ScheduledAction, time.Time) (bool, error)
}

type Scheduler struct{ repository Repository }

func NewScheduler(repository Repository) *Scheduler { return &Scheduler{repository: repository} }

func (s *Scheduler) Schedule(ctx context.Context, snapshot decisioning.Snapshot) (*ScheduledAction, error) {
	if snapshot.Policy.Decision != "APPROVE" {
		return nil, nil
	}
	if snapshot.Gate.Decision != "ALLOW" {
		return nil, errors.New("economically blocked decision may not be scheduled")
	}
	if snapshot.Decision.Optimization.Selected.Action == domain.ActionWait {
		return nil, nil
	}
	return s.repository.ScheduleDecision(ctx, snapshot)
}

type ContextProvider interface {
	Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error)
}

type Reassessor interface {
	Reassess(context.Context, domain.ID) error
}

type Worker struct {
	repository Repository
	contexts   ContextProvider
	registry   *executor.Registry
	reassessor Reassessor
	workerID   string
	now        func() time.Time
}

func NewWorker(repository Repository, contexts ContextProvider, registry *executor.Registry, workerID string) *Worker {
	return &Worker{repository: repository, contexts: contexts, registry: registry, workerID: workerID, now: time.Now}
}

func (w *Worker) SetReassessor(reassessor Reassessor) { w.reassessor = reassessor }

var ErrNoDueWork = errors.New("no due scheduled action")
var ErrNoDueObservation = errors.New("no due outcome observation")

func (w *Worker) RunOnce(ctx context.Context) (*executor.Result, error) {
	now := w.now().UTC()
	scheduled, err := w.repository.ClaimDue(ctx, w.workerID, now, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	decisionContext, err := w.contexts.Get(ctx, scheduled.CaseID)
	if err != nil {
		return nil, err
	}
	if decisionContext.Case.Version != scheduled.CaseVersionAtSchedule || decisionContext.Case.CurrentState == domain.StateRecovered || decisionContext.PaymentState.AlreadyRecovered {
		_ = w.repository.MarkSuppressed(ctx, *scheduled, "STALE_OR_RECOVERED", now)
		return nil, nil
	}
	authorization, err := w.repository.LoadAuthorization(ctx, scheduled.DecisionID)
	if err != nil {
		return nil, err
	}
	if authorization.Gate.Decision != "ALLOW" {
		_ = w.repository.MarkSuppressed(ctx, *scheduled, "ECONOMIC_GATE_BLOCKED", now)
		return nil, nil
	}

	// ScheduleDecision already atomically proved the originating decision fresh,
	// then advanced the aggregate through POLICY_REVIEW and SCHEDULED. The
	// scheduled case-version check above protects against every later mutation.
	// Compare policy to the current scheduled aggregate version here; comparing
	// to the pre-schedule decision version would reject every legitimate action.
	freshPolicy := policy.Evaluate(decisionContext, scheduled.DecisionID, decisionContext.Case.Version, authorization.Candidate, authorization.Gate, now)
	if freshPolicy.Decision != "APPROVE" {
		_ = w.repository.MarkSuppressed(ctx, *scheduled, "POLICY_RECHECK_"+freshPolicy.Decision, now)
		return nil, nil
	}
	// A retry may follow a provider success whose response was lost. Reconcile
	// the stable provider idempotency key before making another external call.
	if scheduled.AttemptCount > 1 {
		if reconciled, reconcileErr := w.registry.Reconcile(ctx, scheduled.Action, scheduled.IdempotencyKey); reconcileErr == nil && reconciled.Status != "" {
			if err = w.repository.MarkExecuting(ctx, *scheduled, now); err != nil {
				return nil, err
			}
			reconciled.ExecutionID = domain.ID(id.New())
			reconciled.Action = scheduled.Action
			reconciled.IdempotencyKey = scheduled.IdempotencyKey
			if err = w.repository.CompleteExecution(ctx, *scheduled, reconciled, w.now().UTC()); err != nil {
				return &reconciled, err
			}
			return &reconciled, nil
		}
	}
	if err = w.repository.MarkExecuting(ctx, *scheduled, now); err != nil {
		return nil, err
	}
	request := executor.Request{
		ExecutionID:        domain.ID(id.New()),
		ScheduledActionID:  scheduled.ID,
		Action:             scheduled.Action,
		IdempotencyKey:     scheduled.IdempotencyKey,
		AmountMinor:        decisionContext.Case.AmountAtRiskMinor,
		Currency:           decisionContext.Case.Currency,
		RecipientReference: string(decisionContext.Case.CustomerID),
		Parameters:         scheduled.Parameters,
	}
	result, executeErr := w.registry.Execute(ctx, request)
	if persistErr := w.repository.CompleteExecution(ctx, *scheduled, result, w.now().UTC()); persistErr != nil {
		return &result, persistErr
	}
	if result.Status == "FAILED" && !result.Retryable && w.reassessor != nil {
		if err := w.reassessor.Reassess(ctx, scheduled.CaseID); err != nil {
			return &result, fmt.Errorf("reassess case: %w", err)
		}
	}
	if executeErr != nil {
		return &result, fmt.Errorf("execute action: %w", executeErr)
	}
	return &result, nil
}

// RunObservationOnce converts a durable, unresolved observation timeout into a
// fresh decision cycle. The repository handles recovered and expired cases.
func (w *Worker) RunObservationOnce(ctx context.Context) error {
	now := w.now().UTC()
	scheduled, err := w.repository.ClaimDueObservation(ctx, w.workerID, now, 2*time.Minute)
	if err != nil {
		return err
	}
	ready, err := w.repository.PrepareReassessment(ctx, *scheduled, now)
	if err != nil || !ready {
		return err
	}
	if w.reassessor == nil {
		return errors.New("reassessor is not configured")
	}
	return w.reassessor.Reassess(ctx, scheduled.CaseID)
}
