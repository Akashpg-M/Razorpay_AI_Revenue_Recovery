package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/recovery"
	"time"
)

const attributionColumns = `id,case_id,recovered_amount_minor,payment_reference,category,decision_id,action_id,execution_id,promise_id,evidence,evidence_strength,rule_version,observed_at,created_at`

func scanAttribution(row rowScanner) (attribution.Record, error) {
	var value attribution.Record
	err := row.Scan(&value.ID, &value.CaseID, &value.RecoveredAmountMinor, &value.PaymentReference, &value.Category, &value.DecisionID, &value.ActionID, &value.ExecutionID, &value.PromiseID, &value.Evidence, &value.EvidenceStrength, &value.RuleVersion, &value.ObservedAt, &value.CreatedAt)
	return value, err
}
func (p *Postgres) ListAttributions(ctx context.Context, caseID domain.ID) ([]attribution.Record, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+attributionColumns+` FROM recovery_attributions WHERE case_id=$1 ORDER BY observed_at,id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []attribution.Record{}
	for rows.Next() {
		value, scanErr := scanAttribution(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func (p *Postgres) AttributeRecovery(ctx context.Context, input attribution.ObserveInput) (attribution.Record, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return attribution.Record{}, false, err
	}
	defer tx.Rollback(ctx)
	var state domain.CaseState
	if err = tx.QueryRow(ctx, `SELECT current_state FROM recovery_cases WHERE id=$1 FOR UPDATE`, input.CaseID).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return attribution.Record{}, false, recovery.ErrNotFound
		}
		return attribution.Record{}, false, err
	}
	existing, existingErr := scanAttribution(tx.QueryRow(ctx, `SELECT `+attributionColumns+` FROM recovery_attributions WHERE case_id=$1 AND payment_reference=$2`, input.CaseID, input.PaymentReference))
	if existingErr == nil {
		_ = tx.Commit(ctx)
		return existing, false, nil
	}
	if !errors.Is(existingErr, pgx.ErrNoRows) {
		return attribution.Record{}, false, existingErr
	}
	var retryWindow, directWindow, emailWindow, ptpWindow int
	err = tx.QueryRow(ctx, `SELECT retry_window_minutes,direct_action_window_minutes,email_assist_window_minutes,ptp_window_minutes FROM attribution_rule_configs WHERE version=$1`, attribution.RuleVersion).Scan(&retryWindow, &directWindow, &emailWindow, &ptpWindow)
	if err != nil {
		return attribution.Record{}, false, err
	}
	_ = emailWindow
	record := attribution.Record{ID: domain.ID(id.New()), CaseID: input.CaseID, RecoveredAmountMinor: input.RecoveredAmountMinor, PaymentReference: input.PaymentReference, Category: attribution.Unknown, EvidenceStrength: "INSUFFICIENT", RuleVersion: attribution.RuleVersion, ObservedAt: input.ObservedAt, CreatedAt: input.ObservedAt.UTC()}
	evidence := map[string]any{"rule_precedence": attribution.Precedence, "overlap_resolution": "highest_precedence_evidence_wins"}
	var exactActionID, exactExecutionID domain.ID
	var exactActionType domain.ActionType
	exactErr := tx.QueryRow(ctx, `SELECT a.id,e.id,a.action_type FROM executions e JOIN recovery_actions a ON a.id=e.action_id WHERE e.case_id=$1 AND e.provider_reference=$2 AND e.status IN('SUCCEEDED','OUTCOME_PENDING','RECOVERY_CONFIRMED') ORDER BY e.completed_at DESC LIMIT 1`, input.CaseID, input.PaymentReference).Scan(&exactActionID, &exactExecutionID, &exactActionType)
	if exactErr == nil {
		record.Category = attribution.DirectAction
		if exactActionType == domain.ActionRetryNow || exactActionType == domain.ActionRetryLater {
			record.Category = attribution.Retry
		}
		record.ActionID = &exactActionID
		record.ExecutionID = &exactExecutionID
		record.EvidenceStrength = "STRONG"
		evidence["matched_provider_reference"] = input.PaymentReference
		evidence["matched_action"] = exactActionType
	} else if !errors.Is(exactErr, pgx.ErrNoRows) {
		return record, false, exactErr
	}
	promise, promiseErr := scanPromise(tx.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE case_id=$1 AND status IN('ACTIVE','FULFILLED') AND due_at BETWEEN $2::timestamptz-make_interval(mins=>$3) AND $2::timestamptz+make_interval(mins=>$3) ORDER BY due_at LIMIT 1`, input.CaseID, input.ObservedAt, ptpWindow))
	if record.Category == attribution.Unknown && promiseErr == nil {
		record.Category = attribution.Promise
		record.PromiseID = &promise.ID
		record.EvidenceStrength = "STRONG"
		evidence["matched_promise_due_at"] = promise.DueAt
	} else if !errors.Is(promiseErr, pgx.ErrNoRows) {
		return record, false, promiseErr
	}
	if record.Category == attribution.Unknown {
		var actionID, executionID domain.ID
		var actionType domain.ActionType
		err = tx.QueryRow(ctx, `SELECT a.id,e.id,a.action_type FROM executions e JOIN recovery_actions a ON a.id=e.action_id WHERE e.case_id=$1 AND e.status IN('SUCCEEDED','OUTCOME_PENDING','RECOVERY_CONFIRMED') AND e.completed_at<=$2::timestamptz AND e.completed_at >= $2::timestamptz-make_interval(mins=>$3) AND a.action_type IN('RETRY_NOW','RETRY_LATER') ORDER BY e.completed_at DESC LIMIT 1`, input.CaseID, input.ObservedAt, retryWindow).Scan(&actionID, &executionID, &actionType)
		if err == nil {
			record.Category = attribution.Retry
			record.ActionID = &actionID
			record.ExecutionID = &executionID
			record.EvidenceStrength = "STRONG"
			evidence["matched_action"] = actionType
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return record, false, err
		}
	}
	if record.Category == attribution.Unknown {
		var actionID, executionID domain.ID
		var actionType domain.ActionType
		err = tx.QueryRow(ctx, `SELECT a.id,e.id,a.action_type FROM executions e JOIN recovery_actions a ON a.id=e.action_id WHERE e.case_id=$1 AND e.status IN('SUCCEEDED','OUTCOME_PENDING','RECOVERY_CONFIRMED') AND e.completed_at<=$2::timestamptz AND e.completed_at >= $2::timestamptz-make_interval(mins=>$3) ORDER BY e.completed_at DESC LIMIT 1`, input.CaseID, input.ObservedAt, directWindow).Scan(&actionID, &executionID, &actionType)
		if err == nil {
			record.Category = attribution.DirectAction
			record.ActionID = &actionID
			record.ExecutionID = &executionID
			record.EvidenceStrength = "MODERATE"
			evidence["matched_action"] = actionType
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return record, false, err
		}
	}
	if record.Category == attribution.Unknown {
		var count int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM executions WHERE case_id=$1 AND completed_at<=$2`, input.CaseID, input.ObservedAt).Scan(&count); err != nil {
			return record, false, err
		}
		if count == 0 {
			record.Category = attribution.Natural
			record.EvidenceStrength = "WEAK"
			evidence["prior_execution_count"] = 0
		}
	}
	var decisionID domain.ID
	decisionErr := tx.QueryRow(ctx, `SELECT id FROM recovery_decisions WHERE case_id=$1 AND created_at<=$2 ORDER BY created_at DESC LIMIT 1`, input.CaseID, input.ObservedAt).Scan(&decisionID)
	if decisionErr == nil {
		record.DecisionID = &decisionID
	} else if !errors.Is(decisionErr, pgx.ErrNoRows) {
		return record, false, decisionErr
	}
	record.Evidence, _ = json.Marshal(evidence)
	_, err = tx.Exec(ctx, `INSERT INTO recovery_attributions(`+attributionColumns+`)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, record.ID, record.CaseID, record.RecoveredAmountMinor, record.PaymentReference, record.Category, record.DecisionID, record.ActionID, record.ExecutionID, record.PromiseID, record.Evidence, record.EvidenceStrength, record.RuleVersion, record.ObservedAt, record.CreatedAt)
	if err != nil {
		return record, false, err
	}
	if record.Category == attribution.Promise && promiseErr == nil && promise.Status == "ACTIVE" {
		_, err = transitionPromiseTx(ctx, tx, promise, "FULFILLED", "ATTRIBUTED_PAYMENT", input.CorrelationID, input.PaymentReference, input.ObservedAt)
		if err != nil {
			return record, false, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='RECOVERED',recovered_amount_minor=recovered_amount_minor+$2,attribution_status=$3,version=version+1,updated_at=$4 WHERE id=$1`, input.CaseID, input.RecoveredAmountMinor, record.Category, input.ObservedAt)
	if err != nil {
		return record, false, err
	}
	feedbackCreated := false
	if record.DecisionID != nil {
		feedbackCreated, err = insertFeedbackForAttribution(ctx, tx, record)
		if err != nil {
			return record, false, err
		}
	}
	sequence, err := nextSequence(ctx, tx, input.CaseID)
	if err != nil {
		return record, false, err
	}
	payload, _ := json.Marshal(record)
	eventTypes := []domain.EventType{domain.EventStateTransitioned, domain.EventRecoveryAttributed, domain.EventRecoveryCompleted}
	if feedbackCreated {
		eventTypes = append(eventTypes, domain.EventFeedbackRecorded)
	}
	for index, eventType := range eventTypes {
		eventPayload := payload
		if eventType == domain.EventStateTransitioned {
			eventPayload, _ = json.Marshal(map[string]any{"from_state": state, "to_state": domain.StateRecovered, "attribution_id": record.ID})
		}
		if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: input.CaseID, Sequence: sequence + int64(index), Type: eventType, Timestamp: input.ObservedAt, Actor: domain.Actor{Type: "SYSTEM", ID: "attribution-engine-v1"}, Payload: eventPayload, CorrelationID: input.CorrelationID}); err != nil {
			return record, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return record, false, err
	}
	return record, true, nil
}

func insertFeedbackForAttribution(ctx context.Context, tx pgx.Tx, record attribution.Record) (bool, error) {
	var caseVersion int64
	var contextVersion, outcomeVersion, naturalVersion, optimizerVersion, selectedAction, policyResult string
	var profileVersion int
	var actionProbability, naturalProbability, uplift float64
	var nerv, cost int64
	var decisionCreated time.Time
	var failure, customerSnapshot, merchantSnapshot json.RawMessage
	err := tx.QueryRow(ctx, `SELECT d.case_version,d.context_version,d.outcome_model_version,d.natural_model_version,d.optimizer_version,d.selected_action,d.merchant_profile_version,dc.action_probability,dc.natural_probability,dc.incremental_uplift,dc.nerv_minor,(dc.channel_cost_minor+dc.incentive_cost_minor+dc.operational_cost_minor),d.created_at,c.failure_or_leak_context,c.customer_context_snapshot,c.merchant_policy_snapshot,COALESCE(pe.result,'UNKNOWN') FROM recovery_decisions d JOIN recovery_decision_candidates dc ON dc.decision_id=d.id AND dc.action=d.selected_action JOIN recovery_cases c ON c.id=d.case_id LEFT JOIN policy_evaluations pe ON pe.decision_id=d.id WHERE d.id=$1`, *record.DecisionID).Scan(&caseVersion, &contextVersion, &outcomeVersion, &naturalVersion, &optimizerVersion, &selectedAction, &profileVersion, &actionProbability, &naturalProbability, &uplift, &nerv, &cost, &decisionCreated, &failure, &customerSnapshot, &merchantSnapshot, &policyResult)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT action FROM recovery_decision_candidates WHERE decision_id=$1 ORDER BY ranking_position`, *record.DecisionID)
	if err != nil {
		return false, err
	}
	eligible := []string{}
	for rows.Next() {
		var action string
		if err = rows.Scan(&action); err != nil {
			rows.Close()
			return false, err
		}
		eligible = append(eligible, action)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	observable, _ := json.Marshal(map[string]any{"failure_or_leak_context": failure, "customer_context_snapshot": customerSnapshot, "merchant_policy_snapshot": merchantSnapshot})
	eligibleJSON, _ := json.Marshal(eligible)
	trainingEligible := record.Category != attribution.Unknown
	exclusions := []string{}
	if !trainingEligible {
		exclusions = append(exclusions, "UNKNOWN_ATTRIBUTION")
	}
	minutes := int64(record.ObservedAt.Sub(decisionCreated).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	_, err = tx.Exec(ctx, `INSERT INTO feedback_records(id,case_id,case_version,decision_id,execution_id,attribution_id,context_version,observable_context,eligible_actions,selected_action,action_probability,natural_probability,incremental_uplift,predicted_nerv_minor,actual_recovered,recovered_amount_minor,intervention_cost_minor,time_to_outcome_minutes,policy_result,outcome_model_version,natural_model_version,optimizer_version,merchant_profile_version,label_version,training_eligible,exclusion_reasons,environment,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,TRUE,$15,$16,$17,$18,$19,$20,$21,$22,'recovery-label-v1',$23,$24,'production',$25)`, id.New(), record.CaseID, caseVersion, record.DecisionID, record.ExecutionID, record.ID, contextVersion, observable, eligibleJSON, selectedAction, actionProbability, naturalProbability, uplift, nerv, record.RecoveredAmountMinor, cost, minutes, policyResult, outcomeVersion, naturalVersion, optimizerVersion, profileVersion, trainingEligible, exclusions, record.CreatedAt)
	return err == nil, err
}
