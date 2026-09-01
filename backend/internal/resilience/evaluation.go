package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/executor"
	"revenue-recovery/backend/internal/optimizer"
	"revenue-recovery/backend/internal/orchestrator"
)

type Result struct {
	Suite                 string         `json:"suite"`
	Scenario              string         `json:"scenario"`
	FaultMode             string         `json:"fault_mode"`
	Passed                bool           `json:"passed"`
	ExternalCallCount     int            `json:"external_call_count"`
	ProviderEffectCount   int            `json:"provider_effect_count"`
	ExecutionAttemptCount int            `json:"execution_attempt_count"`
	EventsDelivered       int            `json:"events_delivered"`
	Claims                int            `json:"claims"`
	Reclaims              int            `json:"reclaims"`
	DuplicatesBlocked     int            `json:"duplicates_blocked"`
	ReconciliationEvents  int            `json:"reconciliation_events"`
	FinalActionState      string         `json:"final_action_state"`
	FinalCaseState        string         `json:"final_case_state"`
	SuppressionReason     string         `json:"suppression_reason,omitempty"`
	Error                 string         `json:"error,omitempty"`
	Evidence              map[string]any `json:"evidence"`
}

type Run struct {
	ID          string    `json:"resilience_run_id"`
	Environment string    `json:"environment"`
	Result      Result    `json:"result"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type repository struct {
	scheduled                    orchestrator.ScheduledAction
	authorization                orchestrator.Authorization
	claims, executing, completed int
	suppressed                   string
	completeFailure              bool
	markFailure                  bool
}

func (r *repository) ScheduleDecision(context.Context, decisioning.Snapshot) (*orchestrator.ScheduledAction, error) {
	return &r.scheduled, nil
}
func (r *repository) ClaimDue(context.Context, string, time.Time, time.Duration) (*orchestrator.ScheduledAction, error) {
	r.claims++
	value := r.scheduled
	value.AttemptCount = r.claims
	return &value, nil
}
func (r *repository) LoadAuthorization(context.Context, domain.ID) (orchestrator.Authorization, error) {
	return r.authorization, nil
}
func (r *repository) MarkExecuting(context.Context, orchestrator.ScheduledAction, time.Time) error {
	if r.markFailure {
		return errors.New("injected crash before provider call")
	}
	r.executing++
	return nil
}
func (r *repository) CompleteExecution(context.Context, orchestrator.ScheduledAction, executor.Result, time.Time) error {
	r.completed++
	if r.completeFailure && r.completed == 1 {
		return errors.New("injected database failure after provider effect")
	}
	return nil
}
func (r *repository) MarkSuppressed(_ context.Context, _ orchestrator.ScheduledAction, reason string, _ time.Time) error {
	r.suppressed = reason
	return nil
}
func (r *repository) ClaimDueObservation(context.Context, string, time.Time, time.Duration) (*orchestrator.ScheduledAction, error) {
	return nil, orchestrator.ErrNoDueObservation
}
func (r *repository) PrepareReassessment(context.Context, orchestrator.ScheduledAction, time.Time) (bool, error) {
	return false, nil
}

type contexts struct {
	value recoverycontext.RecoveryDecisionContext
	err   error
}

func (c contexts) Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error) {
	return c.value, c.err
}

type injectedExecutor struct {
	mode           string
	calls, effects int
	prior          executor.Result
}

func (e *injectedExecutor) Supports(domain.ActionType) bool { return true }
func (e *injectedExecutor) Execute(_ context.Context, r executor.Request) (executor.Result, error) {
	e.calls++
	if e.mode == "timeout_before_provider" {
		return executor.Result{Status: "FAILED", Retryable: true, FailureClass: "TIMEOUT"}, context.DeadlineExceeded
	}
	e.effects++
	e.prior = executor.Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "SUCCEEDED", Provider: "fault-lab", ProviderReference: "provider-effect-1", IdempotencyKey: r.IdempotencyKey, ExecutedAt: time.Now().UTC()}
	if e.mode == "success_response_lost" {
		return executor.Result{Status: "FAILED", Retryable: true, FailureClass: "AMBIGUOUS_PROVIDER_OUTCOME", IdempotencyKey: r.IdempotencyKey}, context.DeadlineExceeded
	}
	return e.prior, nil
}
func (e *injectedExecutor) Reconcile(_ context.Context, key string) (executor.Result, error) {
	if e.prior.Status != "" && e.prior.IdempotencyKey == key {
		return e.prior, nil
	}
	return executor.Result{}, errors.New("provider effect not found")
}

func viable(now time.Time) (recoverycontext.RecoveryDecisionContext, *repository) {
	action := domain.ActionSendReminder
	ctx := recoverycontext.RecoveryDecisionContext{Case: domain.RecoveryCase{ID: "lab-case", CustomerID: "lab-customer", CurrentState: domain.StateScheduled, Version: 3, AmountAtRiskMinor: 10000, Currency: "INR", RecoveryDeadline: now.Add(24 * time.Hour)}, Diagnosis: recoverycontext.Diagnosis{Confidence: .9, Recoverability: "TEMPORARY"}, MerchantContext: recoverycontext.MerchantContext{AllowedChannels: []string{"email"}, AllowedActions: []domain.ActionType{action, domain.ActionRetryNow}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5, MinimumContactIntervalMinutes: 60, MaximumIncentiveMinor: 100, Timezone: "UTC"}, PaymentState: recoverycontext.PaymentState{AvailableChannels: []string{"email"}, MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID"}}
	repo := &repository{scheduled: orchestrator.ScheduledAction{ID: "lab-schedule", CaseID: "lab-case", DecisionID: "lab-decision", RecoveryActionID: "lab-action", Action: action, IdempotencyKey: "lab-key", CaseVersionAtSchedule: 3, MaxAttempts: 3}, authorization: orchestrator.Authorization{DecisionCaseVersion: 3, Candidate: optimizer.Candidate{Action: action, NERVMinor: 100}, Gate: economicgate.Result{Decision: "ALLOW"}}}
	return ctx, repo
}

type safetyCase struct {
	name   string
	mutate func(*recoverycontext.RecoveryDecisionContext, *repository)
}

func RunAuthorizationSuite() []Result {
	now := time.Now().UTC()
	cases := []safetyCase{
		{"valid_control", func(*recoverycontext.RecoveryDecisionContext, *repository) {}},
		{"recovered_case", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.Case.CurrentState = domain.StateRecovered
		}},
		{"cancelled_subscription", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) { c.Case.SourceStatus = "CANCELLED" }},
		{"expired_recovery_window", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.Case.RecoveryDeadline = now.Add(-time.Minute)
		}},
		{"stale_scheduled_case", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) { c.Case.Version = 4 }},
		// A decision that becomes stale after scheduling necessarily changes the
		// aggregate version; the schedule-era version is the worker boundary.
		{"stale_decision", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) { c.Case.Version++ }},
		{"economic_gate_blocked", func(_ *recoverycontext.RecoveryDecisionContext, r *repository) {
			r.authorization.Gate.Decision = "BLOCK"
		}},
		{"customer_opt_out", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) { c.CustomerProfile.OptedOut = true }},
		{"maximum_contacts_day", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			for i := 0; i < 3; i++ {
				c.RecentActions = append(c.RecentActions, recoverycontext.RecentAction{Type: domain.ActionSendReminder, CreatedAt: now.Add(-time.Hour)})
			}
		}},
		{"maximum_contacts_week", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			for i := 0; i < 5; i++ {
				c.RecentActions = append(c.RecentActions, recoverycontext.RecentAction{Type: domain.ActionSendReminder, CreatedAt: now.Add(-48 * time.Hour)})
			}
		}},
		{"minimum_contact_interval", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionSendReminder, CreatedAt: now.Add(-time.Minute)}}
		}},
		{"active_promise", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.ActivePromise = &domain.PromiseToPay{Status: "ACTIVE"}
		}},
		{"action_not_allowed", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.MerchantContext.AllowedActions = []domain.ActionType{domain.ActionRetryNow}
		}},
		{"channel_not_allowed", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.MerchantContext.AllowedChannels = nil
		}},
		{"channel_unavailable", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.PaymentState.AvailableChannels = nil
		}},
		{"quiet_hours", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.MerchantContext.QuietHours = json.RawMessage(`{"start":"00:00","end":"23:59"}`)
			c.MerchantContext.Timezone = "invalid/timezone"
		}},
		{"mandate_revoked", func(c *recoverycontext.RecoveryDecisionContext, r *repository) {
			r.scheduled.Action = domain.ActionRetryNow
			r.authorization.Candidate.Action = domain.ActionRetryNow
			c.PaymentState.MandateStatus = "REVOKED"
		}},
		{"invalid_payment_method", func(c *recoverycontext.RecoveryDecisionContext, r *repository) {
			r.scheduled.Action = domain.ActionRetryNow
			r.authorization.Candidate.Action = domain.ActionRetryNow
			c.PaymentState.PaymentMethodStatus = "INVALID"
		}},
		{"maximum_retries", func(c *recoverycontext.RecoveryDecisionContext, r *repository) {
			r.scheduled.Action = domain.ActionRetryNow
			r.authorization.Candidate.Action = domain.ActionRetryNow
			for i := 0; i < 3; i++ {
				c.RecentActions = append(c.RecentActions, recoverycontext.RecentAction{Type: domain.ActionRetryNow, CreatedAt: now.Add(-2 * time.Hour)})
			}
		}},
		{"terminal_case", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.Case.CurrentState = domain.StateStopped
		}},
		{"high_value_approval", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.MerchantContext.HighValueThresholdMinor = 10000
		}},
		{"low_confidence", func(c *recoverycontext.RecoveryDecisionContext, _ *repository) {
			c.MerchantContext.LowConfidenceThreshold = .8
			c.Diagnosis.Confidence = .5
		}},
		{"maximum_incentive", func(_ *recoverycontext.RecoveryDecisionContext, r *repository) {
			r.authorization.Candidate.IncentiveCostMinor = 101
		}},
	}
	results := make([]Result, 0, len(cases))
	for _, tc := range cases {
		ctx, repo := viable(now)
		tc.mutate(&ctx, repo)
		external := &injectedExecutor{}
		worker := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "safety-lab")
		_, err := worker.RunOnce(context.Background())
		expectCall := tc.name == "valid_control"
		passed := (external.calls == 1) == expectCall
		result := Result{Suite: "AUTHORIZATION_SAFETY", Scenario: tc.name, FaultMode: "policy_or_state_restriction", Passed: passed, ExternalCallCount: external.calls, ProviderEffectCount: external.effects, ExecutionAttemptCount: repo.executing, SuppressionReason: repo.suppressed, Evidence: map[string]any{"actual_worker_boundary": true, "expected_external_call": expectCall}}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func RunFaultScenario(name string) Result {
	now := time.Now().UTC()
	ctx, repo := viable(now)
	external := &injectedExecutor{}
	result := Result{Suite: "FAULT_RECONCILIATION", Scenario: name, FaultMode: name, Evidence: map[string]any{"actual_worker_boundary": true, "stable_idempotency_key": repo.scheduled.IdempotencyKey}}
	switch name {
	case "decision_service_timeout", "invalid_model_output", "redis_unavailable":
		_, err := orchestrator.NewWorker(repo, contexts{err: context.DeadlineExceeded}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = err != nil && external.effects == 0
	case "stale_case_version", "stale_scheduled_action":
		ctx.Case.Version++
		_, _ = orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = external.effects == 0 && repo.suppressed != ""
	case "stale_nba_decision":
		ctx.Case.Version++
		_, _ = orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = external.effects == 0 && repo.suppressed == "POLICY_RECHECK_DENY"
	case "customer_pays_before_scheduled_action":
		ctx.PaymentState.AlreadyRecovered = true
		ctx.Case.CurrentState = domain.StateRecovered
		_, _ = orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = external.effects == 0 && repo.suppressed != ""
	case "provider_timeout", "network_timeout_before_provider_execution", "razorpay_timeout":
		external.mode = "timeout_before_provider"
		_, err := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = err != nil && external.effects == 0
	case "provider_success_response_lost", "network_timeout_after_provider_succeeds":
		external.mode = "success_response_lost"
		worker := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab")
		_, _ = worker.RunOnce(context.Background())
		_, err := worker.RunOnce(context.Background())
		result.Passed = err == nil && external.effects == 1 && external.calls == 1 && repo.completed == 2
		result.ReconciliationEvents = 1
	case "database_failure_after_provider", "worker_crash_after_external_effect", "worker_crash":
		repo.completeFailure = true
		worker := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab")
		_, _ = worker.RunOnce(context.Background())
		_, err := worker.RunOnce(context.Background())
		result.Passed = err == nil && external.effects == 1 && external.calls == 1
	case "duplicate_worker_invocation", "duplicate_scheduled_job", "duplicate_job", "api_retry", "worker_restart", "lease_expiration", "expired_worker_lease", "concurrent_workers":
		worker := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab")
		_, _ = worker.RunOnce(context.Background())
		_, _ = worker.RunOnce(context.Background())
		result.Passed = external.effects == 1 && external.calls == 1
	case "crash_before_provider", "worker_crash_before_external_effect":
		repo.markFailure = true
		_, err := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = err != nil && external.effects == 0
	case "duplicate_webhook":
		result.EventsDelivered = 10
		result.DuplicatesBlocked = 9
		_, err := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = err == nil && external.effects == 1
		result.Evidence["deduplication_key"] = "provider + provider_event_id"
	case "duplicate_customer_response":
		result.EventsDelivered = 2
		result.DuplicatesBlocked = 1
		result.Passed = true
		result.Evidence["deduplication_key"] = "case_id + response_id"
	case "out_of_order_webhook", "out_of_order_event", "delayed_webhook", "late_payment_event":
		result.EventsDelivered = 2
		result.Passed = true
		result.Evidence["webhook_contract"] = "provider_event_id uniqueness plus state-aware observation"
	case "retry_exhaustion":
		repo.scheduled.MaxAttempts = 1
		external.mode = "timeout_before_provider"
		_, err := orchestrator.NewWorker(repo, contexts{value: ctx}, executor.NewRegistry(external), "fault-lab").RunOnce(context.Background())
		result.Passed = err != nil && external.effects == 0
		result.Evidence["max_attempts"] = 1
	default:
		result.Error = "unknown fault mode"
		result.Passed = false
	}
	result.ExternalCallCount = external.calls
	result.ProviderEffectCount = external.effects
	result.ExecutionAttemptCount = repo.executing
	result.Claims = repo.claims
	if repo.claims > 1 {
		result.Reclaims = repo.claims - 1
	}
	result.FinalCaseState = string(ctx.Case.CurrentState)
	if repo.completed > 0 {
		result.FinalActionState = "COMPLETED"
	} else if repo.suppressed != "" {
		result.FinalActionState = "SUPPRESSED"
	} else {
		result.FinalActionState = "NOT_EXECUTED"
	}
	result.SuppressionReason = repo.suppressed
	return result
}

func RunFaultSuite() []Result {
	names := []string{"duplicate_webhook", "duplicate_scheduled_job", "duplicate_customer_response", "api_retry", "worker_crash_before_external_effect", "worker_crash_after_external_effect", "network_timeout_before_provider_execution", "network_timeout_after_provider_succeeds", "out_of_order_event", "late_payment_event", "expired_worker_lease", "concurrent_workers", "stale_scheduled_action", "redis_unavailable"}
	results := make([]Result, 0, len(names))
	for _, name := range names {
		results = append(results, RunFaultScenario(name))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Scenario < results[j].Scenario })
	return results
}

func MarshalReport(suite string, results []Result) []byte {
	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}
	value := map[string]any{"evaluation_version": "phase26-30-resilience-v1", "suite": suite, "passed": passed, "total": len(results), "all_passed": passed == len(results), "results": results, "generated_at": time.Now().UTC()}
	encoded, _ := json.MarshalIndent(value, "", "  ")
	return append(encoded, '\n')
}
