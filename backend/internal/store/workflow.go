package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/executor"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/recovery"
)

func (p *Postgres) ScheduleDecision(ctx context.Context, snapshot decisioning.Snapshot) (*orchestrator.ScheduledAction, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	scheduled, err := scheduleDecisionTx(ctx, tx, snapshot)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return scheduled, nil
}

func scheduleDecisionTx(ctx context.Context, tx pgx.Tx, snapshot decisioning.Snapshot) (*orchestrator.ScheduledAction, error) {
	var err error
	var state domain.CaseState
	var version int64
	if err = tx.QueryRow(ctx, `SELECT current_state,version FROM recovery_cases WHERE id=$1 FOR UPDATE`, snapshot.Decision.CaseID).Scan(&state, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, recovery.ErrNotFound
		}
		return nil, err
	}
	if state != domain.StateActionPending || version != snapshot.Decision.CaseVersion {
		return nil, recovery.ErrConflict
	}

	now := time.Now().UTC()
	scheduledFor := now
	if snapshot.Decision.Optimization.Selected.Action == domain.ActionRetryLater {
		scheduledFor = now.Add(24 * time.Hour)
	}
	actionID := domain.ID(id.New())
	scheduledID := domain.ID(id.New())
	idempotencyKey := "scheduled-action:" + string(scheduledID)
	parameters, _ := json.Marshal(map[string]any{"decision_id": snapshot.Decision.ID})

	_, err = tx.Exec(ctx, `INSERT INTO recovery_actions(id,case_id,action_type,status,parameters,idempotency_key,scheduled_at,created_at,updated_at) VALUES($1,$2,$3,'SCHEDULED',$4,$5,$6,$7,$7)`, actionID, snapshot.Decision.CaseID, snapshot.Decision.Optimization.Selected.Action, parameters, idempotencyKey, scheduledFor, now)
	if err != nil {
		return nil, err
	}
	// Persist both valid state transitions; no implicit ACTION_PENDING -> SCHEDULED jump.
	command, err := tx.Exec(ctx, `UPDATE recovery_cases SET current_state='POLICY_REVIEW',version=version+1,updated_at=$2 WHERE id=$1 AND current_state='ACTION_PENDING' AND version=$3`, snapshot.Decision.CaseID, now, version)
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() != 1 {
		return nil, recovery.ErrConflict
	}
	version++
	command, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='SCHEDULED',version=version+1,updated_at=$2 WHERE id=$1 AND current_state='POLICY_REVIEW' AND version=$3`, snapshot.Decision.CaseID, now, version)
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() != 1 {
		return nil, recovery.ErrConflict
	}
	version++

	_, err = tx.Exec(ctx, `INSERT INTO scheduled_actions(id,case_id,decision_id,policy_evaluation_id,recovery_action_id,action,parameters,scheduled_for,status,max_attempts,idempotency_key,case_version_at_schedule,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',3,$9,$10,$11)`, scheduledID, snapshot.Decision.CaseID, snapshot.Decision.ID, snapshot.Policy.ID, actionID, snapshot.Decision.Optimization.Selected.Action, parameters, scheduledFor, idempotencyKey, version, now)
	if err != nil {
		return nil, err
	}

	sequence, err := nextSequence(ctx, tx, snapshot.Decision.CaseID)
	if err != nil {
		return nil, err
	}
	correlationID := string(scheduledID)
	policyReviewPayload, _ := json.Marshal(map[string]any{"from_state": domain.StateActionPending, "to_state": domain.StatePolicyReview, "decision_id": snapshot.Decision.ID})
	scheduledStatePayload, _ := json.Marshal(map[string]any{"from_state": domain.StatePolicyReview, "to_state": domain.StateScheduled, "decision_id": snapshot.Decision.ID, "scheduled_action_id": scheduledID})
	payload, _ := json.Marshal(map[string]any{"decision_id": snapshot.Decision.ID, "scheduled_action_id": scheduledID, "action": snapshot.Decision.Optimization.Selected.Action, "scheduled_for": scheduledFor})
	events := []domain.RecoveryEvent{
		{ID: domain.ID(id.New()), CaseID: snapshot.Decision.CaseID, Sequence: sequence, Type: domain.EventStateTransitioned, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "durable-scheduler-v1"}, Payload: policyReviewPayload, CorrelationID: correlationID},
		{ID: domain.ID(id.New()), CaseID: snapshot.Decision.CaseID, Sequence: sequence + 1, Type: domain.EventStateTransitioned, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "durable-scheduler-v1"}, Payload: scheduledStatePayload, CorrelationID: correlationID},
		{ID: domain.ID(id.New()), CaseID: snapshot.Decision.CaseID, Sequence: sequence + 2, Type: domain.EventActionScheduled, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "durable-scheduler-v1"}, Payload: payload, CorrelationID: correlationID},
	}
	for _, event := range events {
		if err = insertEvent(ctx, tx, event); err != nil {
			return nil, err
		}
	}
	return &orchestrator.ScheduledAction{ID: scheduledID, CaseID: snapshot.Decision.CaseID, DecisionID: snapshot.Decision.ID, PolicyEvaluationID: snapshot.Policy.ID, RecoveryActionID: actionID, Action: snapshot.Decision.Optimization.Selected.Action, Parameters: parameters, ScheduledFor: scheduledFor, Status: "PENDING", MaxAttempts: 3, IdempotencyKey: idempotencyKey, CaseVersionAtSchedule: version}, nil
}

func (p *Postgres) ClaimDue(ctx context.Context, workerID string, now time.Time, lease time.Duration) (*orchestrator.ScheduledAction, error) {
	row := p.pool.QueryRow(ctx, `WITH due AS (
		SELECT id FROM scheduled_actions
		WHERE status IN('PENDING','RETRY_PENDING','CLAIMED','EXECUTING')
		  AND COALESCE(next_retry_at,scheduled_for) <= $1
		  AND (status NOT IN('CLAIMED','EXECUTING') OR lease_expires_at <= $1)
		ORDER BY COALESCE(next_retry_at,scheduled_for),id
		FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE scheduled_actions s SET status='CLAIMED',lease_owner=$2,lease_expires_at=$3,attempt_count=attempt_count+1
	FROM due WHERE s.id=due.id
	RETURNING s.id,s.case_id,s.decision_id,s.policy_evaluation_id,s.recovery_action_id,s.action,s.parameters,s.scheduled_for,s.status,s.attempt_count,s.max_attempts,s.idempotency_key,s.case_version_at_schedule,s.next_retry_at`, now, workerID, now.Add(lease))
	var scheduled orchestrator.ScheduledAction
	err := row.Scan(&scheduled.ID, &scheduled.CaseID, &scheduled.DecisionID, &scheduled.PolicyEvaluationID, &scheduled.RecoveryActionID, &scheduled.Action, &scheduled.Parameters, &scheduled.ScheduledFor, &scheduled.Status, &scheduled.AttemptCount, &scheduled.MaxAttempts, &scheduled.IdempotencyKey, &scheduled.CaseVersionAtSchedule, &scheduled.NextRetryAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, orchestrator.ErrNoDueWork
	}
	return &scheduled, err
}

func (p *Postgres) LoadAuthorization(ctx context.Context, decisionID, policyEvaluationID domain.ID) (orchestrator.Authorization, error) {
	var result orchestrator.Authorization
	err := p.pool.QueryRow(ctx, `SELECT d.case_version,c.action,c.action_probability,c.natural_probability,c.incremental_uplift,c.gross_incremental_value_minor,c.channel_cost_minor,c.incentive_cost_minor,c.operational_cost_minor,c.fatigue_penalty_minor,c.risk_penalty_minor,c.nerv_minor,c.objective_score_minor,c.ranking_position,c.reason_codes,g.id,g.case_id,g.action,g.nerv_minor,g.threshold_minor,g.result,g.reason_code,g.gate_version,g.created_at,
	EXISTS(SELECT 1 FROM policy_evaluations pe WHERE pe.id=$2 AND pe.result='APPROVE' AND 'HUMAN_APPROVAL_SATISFIED'=ANY(pe.reason_codes))
	FROM recovery_decisions d JOIN recovery_decision_candidates c ON c.decision_id=d.id AND c.action=d.selected_action JOIN economic_gate_evaluations g ON g.decision_id=d.id WHERE d.id=$1`, decisionID, policyEvaluationID).Scan(
		&result.DecisionCaseVersion, &result.Candidate.Action, &result.Candidate.ActionRecoveryProbability, &result.Candidate.NaturalRecoveryProbability, &result.Candidate.IncrementalUplift, &result.Candidate.GrossIncrementalValueMinor, &result.Candidate.ChannelCostMinor, &result.Candidate.IncentiveCostMinor, &result.Candidate.OperationalCostMinor, &result.Candidate.FatiguePenaltyMinor, &result.Candidate.RiskPenaltyMinor, &result.Candidate.NERVMinor, &result.Candidate.ObjectiveScoreMinor, &result.Candidate.Rank, &result.Candidate.ReasonCodes,
		&result.Gate.ID, &result.Gate.CaseID, &result.Gate.Action, &result.Gate.NERVMinor, &result.Gate.ThresholdMinor, &result.Gate.Decision, &result.Gate.Reason, &result.Gate.GateVersion, &result.Gate.CreatedAt, &result.HumanApproved)
	result.Gate.DecisionID = decisionID
	return result, err
}

func (p *Postgres) MarkExecuting(ctx context.Context, scheduled orchestrator.ScheduledAction, now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE scheduled_actions SET status='EXECUTING',started_at=COALESCE(started_at,$2) WHERE id=$1 AND status='CLAIMED'`, scheduled.ID, now)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return recovery.ErrConflict
	}
	// The first attempt advances SCHEDULED -> EXECUTING. Retries retain EXECUTING.
	var current domain.CaseState
	var version int64
	if err = tx.QueryRow(ctx, `SELECT current_state,version FROM recovery_cases WHERE id=$1 FOR UPDATE`, scheduled.CaseID).Scan(&current, &version); err != nil {
		return err
	}
	if current == domain.StateScheduled {
		if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='EXECUTING',version=version+1,updated_at=$2 WHERE id=$1`, scheduled.CaseID, now); err != nil {
			return err
		}
		version++
		if _, err = tx.Exec(ctx, `UPDATE scheduled_actions SET case_version_at_schedule=$2 WHERE id=$1`, scheduled.ID, version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) CompleteExecution(ctx context.Context, scheduled orchestrator.ScheduledAction, result executor.Result, now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status := result.Status
	if status != "SUCCEEDED" && status != "OUTCOME_PENDING" {
		status = "FAILED"
	}
	response, _ := json.Marshal(result)
	_, err = tx.Exec(ctx, `INSERT INTO executions(id,case_id,action_id,attempt,status,provider_reference,request,response,started_at,completed_at,scheduled_action_id,idempotency_key,failure_class,retryable) VALUES($1,$2,$3,$4,$5,$6,'{}',$7,$8,$8,$9,$10,$11,$12) ON CONFLICT(idempotency_key) DO NOTHING`, result.ExecutionID, scheduled.CaseID, scheduled.RecoveryActionID, scheduled.AttemptCount, status, result.ProviderReference, response, now, scheduled.ID, fmt.Sprintf("%s:%d", scheduled.IdempotencyKey, scheduled.AttemptCount), result.FailureClass, result.Retryable)
	if err != nil {
		return err
	}

	if status == "FAILED" && result.Retryable && scheduled.AttemptCount < scheduled.MaxAttempts {
		delay := time.Duration(1<<min(scheduled.AttemptCount-1, 6)) * time.Minute
		_, err = tx.Exec(ctx, `UPDATE scheduled_actions SET status='RETRY_PENDING',next_retry_at=$2,lease_owner=NULL,lease_expires_at=NULL,failure_reason=$3 WHERE id=$1`, scheduled.ID, now.Add(delay), result.FailureClass)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	finalScheduledStatus := "OBSERVATION_PENDING"
	actionStatus := "EXECUTED"
	eventType := domain.EventActionExecuted
	if status == "FAILED" {
		finalScheduledStatus = "FAILED"
		actionStatus = "FAILED"
		eventType = domain.EventActionFailed
	}
	observationAt := now.Add(24 * time.Hour)
	var observationDue *time.Time
	if status != "FAILED" {
		observationDue = &observationAt
	}
	_, err = tx.Exec(ctx, `UPDATE scheduled_actions SET status=$2,completed_at=$3,next_retry_at=$5,lease_owner=NULL,lease_expires_at=NULL,failure_reason=NULLIF($4,'') WHERE id=$1`, scheduled.ID, finalScheduledStatus, now, result.FailureClass, observationDue)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE recovery_actions SET status=$2,updated_at=$3 WHERE id=$1`, scheduled.RecoveryActionID, actionStatus, now)
	if err != nil {
		return err
	}

	var current domain.CaseState
	var version int64
	if err = tx.QueryRow(ctx, `SELECT current_state,version FROM recovery_cases WHERE id=$1 FOR UPDATE`, scheduled.CaseID).Scan(&current, &version); err != nil {
		return err
	}
	if current == domain.StateExecuting {
		target := domain.StateWaitingOutcome
		if status == "FAILED" {
			target = domain.StateReassessing
		}
		if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state=$2,version=version+1,updated_at=$3 WHERE id=$1`, scheduled.CaseID, target, now); err != nil {
			return err
		}
		version++
		if status == "FAILED" {
			if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='ACTION_PENDING',version=version+1,updated_at=$2 WHERE id=$1`, scheduled.CaseID, now); err != nil {
				return err
			}
			version++
		}
	}
	sequence, err := nextSequence(ctx, tx, scheduled.CaseID)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"scheduled_action_id": scheduled.ID, "execution_id": result.ExecutionID, "status": status, "failure_class": result.FailureClass, "retryable": result.Retryable})
	if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: scheduled.CaseID, Sequence: sequence, Type: eventType, Timestamp: now, Actor: domain.Actor{Type: "WORKER", ID: "durable-worker-v1"}, Payload: payload, CorrelationID: string(scheduled.ID)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) MarkSuppressed(ctx context.Context, scheduled orchestrator.ScheduledAction, reason string, now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE scheduled_actions SET status='SUPERSEDED',failure_reason=$2,completed_at=$3,lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND status IN('CLAIMED','PENDING','RETRY_PENDING')`, scheduled.ID, reason, now)
	if err != nil {
		return err
	}
	sequence, err := nextSequence(ctx, tx, scheduled.CaseID)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"scheduled_action_id": scheduled.ID, "reason": reason})
	if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: scheduled.CaseID, Sequence: sequence, Type: domain.EventActionSuppressed, Timestamp: now, Actor: domain.Actor{Type: "WORKER", ID: "durable-worker-v1"}, Payload: payload, CorrelationID: string(scheduled.ID)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ClaimDueObservation(ctx context.Context, workerID string, now time.Time, lease time.Duration) (*orchestrator.ScheduledAction, error) {
	row := p.pool.QueryRow(ctx, `WITH due AS (SELECT id FROM scheduled_actions WHERE status IN('OBSERVATION_PENDING','OBSERVATION_CLAIMED') AND next_retry_at <= $1 AND (status <> 'OBSERVATION_CLAIMED' OR lease_expires_at <= $1) ORDER BY next_retry_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE scheduled_actions s SET status='OBSERVATION_CLAIMED',lease_owner=$2,lease_expires_at=$3 FROM due WHERE s.id=due.id RETURNING s.id,s.case_id,s.decision_id,s.policy_evaluation_id,s.recovery_action_id,s.action,s.parameters,s.scheduled_for,s.status,s.attempt_count,s.max_attempts,s.idempotency_key,s.case_version_at_schedule,s.next_retry_at`, now, workerID, now.Add(lease))
	var scheduled orchestrator.ScheduledAction
	err := row.Scan(&scheduled.ID, &scheduled.CaseID, &scheduled.DecisionID, &scheduled.PolicyEvaluationID, &scheduled.RecoveryActionID, &scheduled.Action, &scheduled.Parameters, &scheduled.ScheduledFor, &scheduled.Status, &scheduled.AttemptCount, &scheduled.MaxAttempts, &scheduled.IdempotencyKey, &scheduled.CaseVersionAtSchedule, &scheduled.NextRetryAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, orchestrator.ErrNoDueObservation
	}
	return &scheduled, err
}

func (p *Postgres) PrepareReassessment(ctx context.Context, scheduled orchestrator.ScheduledAction, now time.Time) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var state domain.CaseState
	var deadline time.Time
	if err = tx.QueryRow(ctx, `SELECT current_state,recovery_deadline FROM recovery_cases WHERE id=$1 FOR UPDATE`, scheduled.CaseID).Scan(&state, &deadline); err != nil {
		return false, err
	}
	ready := false
	outcome := "NO_RECOVERY_OBSERVED"
	if state == domain.StateWaitingOutcome {
		if !deadline.After(now) {
			_, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='EXHAUSTED',version=version+1,updated_at=$2 WHERE id=$1`, scheduled.CaseID, now)
			outcome = "RECOVERY_WINDOW_EXPIRED"
		} else {
			_, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='REASSESSING',version=version+1,updated_at=$2 WHERE id=$1`, scheduled.CaseID, now)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='ACTION_PENDING',version=version+1,updated_at=$2 WHERE id=$1`, scheduled.CaseID, now)
				ready = err == nil
			}
		}
		if err != nil {
			return false, err
		}
	} else if state == domain.StateRecovered {
		outcome = "RECOVERED_BEFORE_TIMEOUT"
	}
	_, err = tx.Exec(ctx, `UPDATE scheduled_actions SET status='OBSERVED',lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND status='OBSERVATION_CLAIMED'`, scheduled.ID)
	if err != nil {
		return false, err
	}
	sequence, err := nextSequence(ctx, tx, scheduled.CaseID)
	if err != nil {
		return false, err
	}
	payload, _ := json.Marshal(map[string]any{"scheduled_action_id": scheduled.ID, "outcome": outcome, "reassessment_ready": ready})
	if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: scheduled.CaseID, Sequence: sequence, Type: domain.EventOutcomeObserved, Timestamp: now, Actor: domain.Actor{Type: "WORKER", ID: "outcome-observer-v1"}, Payload: payload, CorrelationID: string(scheduled.ID)}); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return ready, nil
}

func (p *Postgres) CaptureEmail(ctx context.Context, request executor.Request, template string, safePayload json.RawMessage) (string, bool, error) {
	deliveryID := id.New()
	command, err := p.pool.Exec(ctx, `INSERT INTO email_deliveries(id,scheduled_action_id,idempotency_key,recipient_reference,template_name,safe_payload,status,created_at) VALUES($1,$2,$3,$4,$5,$6,'CAPTURED',$7) ON CONFLICT(idempotency_key) DO NOTHING`, deliveryID, request.ScheduledActionID, request.IdempotencyKey, request.RecipientReference, template, safePayload, time.Now().UTC())
	if err != nil {
		return "", false, err
	}
	if command.RowsAffected() == 1 {
		return deliveryID, true, nil
	}
	var existing string
	err = p.pool.QueryRow(ctx, `SELECT id FROM email_deliveries WHERE idempotency_key=$1`, request.IdempotencyKey).Scan(&existing)
	return existing, false, err
}

func (p *Postgres) ProviderName() string { return "local-retry-capture" }

func (p *Postgres) RequestRetry(ctx context.Context, idempotencyKey string, payload json.RawMessage) (string, error) {
	requestID := id.New()
	command, err := p.pool.Exec(ctx, `INSERT INTO retry_requests(id,idempotency_key,payload,status,created_at) VALUES($1,$2,$3,'CAPTURED',$4) ON CONFLICT(idempotency_key) DO NOTHING`, requestID, idempotencyKey, payload, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if command.RowsAffected() == 1 {
		return requestID, nil
	}
	var existing string
	err = p.pool.QueryRow(ctx, `SELECT id FROM retry_requests WHERE idempotency_key=$1`, idempotencyKey).Scan(&existing)
	return existing, err
}
