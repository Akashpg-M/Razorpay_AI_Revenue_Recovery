package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/orchestrator"
)

func (p *Postgres) GetWorkflow(ctx context.Context, caseID domain.ID) (orchestrator.WorkflowView, error) {
	recoveryCase, err := p.GetCase(ctx, caseID)
	if err != nil {
		return orchestrator.WorkflowView{}, err
	}
	view := orchestrator.WorkflowView{Case: recoveryCase, ScheduledActions: []orchestrator.ScheduledAction{}, Executions: []json.RawMessage{}}
	err = p.pool.QueryRow(ctx, `SELECT row_to_json(d)::jsonb FROM recovery_decisions d WHERE case_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, caseID).Scan(&view.LatestDecision)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return view, err
	}
	err = p.pool.QueryRow(ctx, `SELECT row_to_json(e)::jsonb FROM policy_evaluations e WHERE case_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, caseID).Scan(&view.LatestPolicy)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return view, err
	}
	rows, err := p.pool.Query(ctx, `SELECT id,case_id,decision_id,policy_evaluation_id,recovery_action_id,action,parameters,scheduled_for,status,attempt_count,max_attempts,idempotency_key,case_version_at_schedule,next_retry_at FROM scheduled_actions WHERE case_id=$1 ORDER BY created_at,id`, caseID)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var item orchestrator.ScheduledAction
		if err = rows.Scan(&item.ID, &item.CaseID, &item.DecisionID, &item.PolicyEvaluationID, &item.RecoveryActionID, &item.Action, &item.Parameters, &item.ScheduledFor, &item.Status, &item.AttemptCount, &item.MaxAttempts, &item.IdempotencyKey, &item.CaseVersionAtSchedule, &item.NextRetryAt); err != nil {
			return view, err
		}
		view.ScheduledActions = append(view.ScheduledActions, item)
	}
	if err = rows.Err(); err != nil {
		return view, err
	}
	executionRows, err := p.pool.Query(ctx, `SELECT row_to_json(e)::jsonb FROM executions e WHERE case_id=$1 ORDER BY started_at,id`, caseID)
	if err != nil {
		return view, err
	}
	defer executionRows.Close()
	for executionRows.Next() {
		var raw json.RawMessage
		if err = executionRows.Scan(&raw); err != nil {
			return view, err
		}
		view.Executions = append(view.Executions, raw)
	}
	return view, executionRows.Err()
}
