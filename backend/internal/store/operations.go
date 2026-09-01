package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/operations"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/recovery"
)

const queueSQL = `SELECT c.id,c.version,c.current_state,
 CASE WHEN c.amount_at_risk_minor>=COALESCE(NULLIF(mp.high_value_threshold_minor,0),9223372036854775807) THEN 'CRITICAL' WHEN c.recovery_deadline<=NOW()+interval '6 hours' THEN 'HIGH' WHEN d.selected_nerv_minor>=100000 THEN 'HIGH' ELSE 'NORMAL' END,
 CASE WHEN 'HIGH_VALUE_APPROVAL'=ANY(pe.reason_codes) THEN 'HIGH_VALUE_APPROVAL' WHEN 'LOW_CONFIDENCE_ESCALATION'=ANY(pe.reason_codes) THEN 'LOW_CONFIDENCE' WHEN EXISTS(SELECT 1 FROM promises_to_pay p WHERE p.case_id=c.id AND p.status='BROKEN') THEN 'BROKEN_PTP' WHEN c.current_state='EXHAUSTED' THEN 'EXHAUSTED' ELSE 'POLICY_ESCALATION' END,
 c.amount_at_risk_minor,c.currency,c.merchant_id,m.name,mp.version,'customer-'||RIGHT(c.customer_id,8),c.leak_type,
 d.id,pe.id,g.id,d.selected_action,d.selected_nerv_minor,dc.action_probability,dc.natural_probability,dc.incremental_uplift,
 CASE WHEN 'LOW_CONFIDENCE_ESCALATION'=ANY(pe.reason_codes) THEN LEAST(mp.low_confidence_threshold,0.5) ELSE 0.9 END,
 pe.reason_codes,d.merchant_objective,c.recovery_deadline,pe.created_at,
 CASE WHEN hr.decision='DEFER' AND hr.review_after<=NOW() THEN 'PENDING' WHEN hr.decision='DEFER' THEN 'DEFERRED' WHEN hr.decision IS NULL THEN 'PENDING' ELSE hr.decision::text END,
 hr.review_after,
 dc.action,dc.action_probability,dc.natural_probability,dc.incremental_uplift,dc.gross_incremental_value_minor,dc.channel_cost_minor,dc.incentive_cost_minor,dc.operational_cost_minor,dc.fatigue_penalty_minor,dc.risk_penalty_minor,dc.nerv_minor,dc.objective_score_minor,dc.ranking_position,dc.reason_codes,
 g.nerv_minor,g.threshold_minor,g.result,g.reason_code,g.gate_version,g.created_at
 FROM recovery_cases c JOIN merchants m ON m.id=c.merchant_id
 JOIN LATERAL(SELECT * FROM merchant_policies x WHERE x.merchant_id=c.merchant_id ORDER BY version DESC LIMIT 1)mp ON TRUE
 JOIN LATERAL(SELECT * FROM recovery_decisions x WHERE x.case_id=c.id ORDER BY created_at DESC,id DESC LIMIT 1)d ON TRUE
 JOIN recovery_decision_candidates dc ON dc.decision_id=d.id AND dc.action=d.selected_action
 JOIN LATERAL(SELECT * FROM policy_evaluations x WHERE x.decision_id=d.id ORDER BY created_at DESC,id DESC LIMIT 1)pe ON TRUE
 JOIN economic_gate_evaluations g ON g.id=pe.economic_gate_id
 LEFT JOIN LATERAL(SELECT * FROM human_review_records x WHERE x.case_id=c.id ORDER BY created_at DESC,id DESC LIMIT 1)hr ON TRUE`

func scanQueueItem(row rowScanner) (operations.QueueItem, error) {
	var item operations.QueueItem
	err := row.Scan(&item.CaseID, &item.CaseVersion, &item.State, &item.Priority, &item.Category, &item.AmountAtRiskMinor, &item.Currency, &item.MerchantID, &item.MerchantName, &item.MerchantPolicyVersion, &item.CustomerSafeReference, &item.LeakType, &item.DecisionID, &item.PolicyEvaluationID, &item.EconomicGateID, &item.RecommendedAction, &item.ExpectedNERVMinor, &item.ActionRecoveryProbability, &item.NaturalRecoveryProbability, &item.IncrementalUplift, &item.DiagnosisConfidence, &item.EscalationReasons, &item.MerchantObjective, &item.RecoveryDeadline, &item.EscalatedAt, &item.ReviewStatus, &item.ReviewAfter, &item.Candidate.Action, &item.Candidate.ActionRecoveryProbability, &item.Candidate.NaturalRecoveryProbability, &item.Candidate.IncrementalUplift, &item.Candidate.GrossIncrementalValueMinor, &item.Candidate.ChannelCostMinor, &item.Candidate.IncentiveCostMinor, &item.Candidate.OperationalCostMinor, &item.Candidate.FatiguePenaltyMinor, &item.Candidate.RiskPenaltyMinor, &item.Candidate.NERVMinor, &item.Candidate.ObjectiveScoreMinor, &item.Candidate.Rank, &item.Candidate.ReasonCodes, &item.Gate.NERVMinor, &item.Gate.ThresholdMinor, &item.Gate.Decision, &item.Gate.Reason, &item.Gate.GateVersion, &item.Gate.CreatedAt)
	item.Gate.ID = item.EconomicGateID
	item.Gate.DecisionID = item.DecisionID
	item.Gate.CaseID = item.CaseID
	item.Gate.Action = item.RecommendedAction
	return item, err
}

func (p *Postgres) ListOperationsQueue(ctx context.Context) ([]operations.QueueItem, error) {
	rows, err := p.pool.Query(ctx, queueSQL+` WHERE c.current_state IN('ESCALATED','EXHAUSTED') ORDER BY c.recovery_deadline,d.selected_nerv_minor DESC,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []operations.QueueItem{}
	for rows.Next() {
		item, scanErr := scanQueueItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (p *Postgres) GetOperationsQueueItem(ctx context.Context, caseID domain.ID) (operations.QueueItem, []operations.Review, error) {
	item, err := scanQueueItem(p.pool.QueryRow(ctx, queueSQL+` WHERE c.id=$1`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, nil, recovery.ErrNotFound
	}
	if err != nil {
		return item, nil, err
	}
	reviews, err := p.listReviews(ctx, caseID)
	return item, reviews, err
}

const reviewColumns = `id,case_id,decision_id,policy_evaluation_id,recommended_action,operator_id,actor_type,actor_metadata,decision,reason_code,notes,case_version_at_review,merchant_policy_version_at_review,review_after,idempotency_key,reauthorization_result,reauthorization_reason_codes,scheduled_action_id,created_at`

func scanReview(row rowScanner) (operations.Review, error) {
	var value operations.Review
	err := row.Scan(&value.ID, &value.CaseID, &value.DecisionID, &value.PolicyEvaluationID, &value.RecommendedAction, &value.OperatorID, &value.ActorType, &value.ActorMetadata, &value.Decision, &value.ReasonCode, &value.Notes, &value.CaseVersionAtReview, &value.MerchantPolicyVersionAtReview, &value.ReviewAfter, &value.IdempotencyKey, &value.ReauthorizationResult, &value.ReauthorizationReasonCodes, &value.ScheduledActionID, &value.CreatedAt)
	return value, err
}
func (p *Postgres) listReviews(ctx context.Context, caseID domain.ID) ([]operations.Review, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+reviewColumns+` FROM human_review_records WHERE case_id=$1 ORDER BY created_at,id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []operations.Review{}
	for rows.Next() {
		value, scanErr := scanReview(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) ApplyHumanReview(ctx context.Context, command operations.ApplyCommand) (operations.Review, *orchestrator.ScheduledAction, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return operations.Review{}, nil, false, err
	}
	defer tx.Rollback(ctx)
	existing, existingErr := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM human_review_records WHERE idempotency_key=$1`, command.Input.IdempotencyKey))
	if existingErr == nil {
		_ = tx.Commit(ctx)
		return existing, nil, false, nil
	}
	if !errors.Is(existingErr, pgx.ErrNoRows) {
		return operations.Review{}, nil, false, existingErr
	}
	var state domain.CaseState
	var version int64
	var policyVersion int
	if err = tx.QueryRow(ctx, `SELECT c.current_state,c.version,(SELECT version FROM merchant_policies WHERE merchant_id=c.merchant_id ORDER BY version DESC LIMIT 1) FROM recovery_cases c WHERE c.id=$1 FOR UPDATE`, command.Item.CaseID).Scan(&state, &version, &policyVersion); err != nil {
		return operations.Review{}, nil, false, err
	}
	reauth := "NOT_REQUIRED"
	reasons := []string{}
	canSchedule := false
	if command.Input.Decision == operations.Approve {
		reauth, reasons, canSchedule = operations.AuthorizeApproval(state, version, command.Input.ExpectedCaseVersion, policyVersion, command.Item.MerchantPolicyVersion, command.Item.RecoveryDeadline, command.Now, command.FreshPolicy)
	}
	if command.Input.Decision != operations.Approve && state != domain.StateEscalated {
		return operations.Review{}, nil, false, operations.ErrNotReviewable
	}
	review := operations.Review{ID: id.New(), CaseID: command.Item.CaseID, DecisionID: command.Item.DecisionID, PolicyEvaluationID: command.Item.PolicyEvaluationID, RecommendedAction: command.Item.RecommendedAction, OperatorID: command.Input.OperatorID, ActorType: command.Input.ActorType, ActorMetadata: command.Input.ActorMetadata, Decision: command.Input.Decision, ReasonCode: command.Input.ReasonCode, Notes: command.Input.Notes, CaseVersionAtReview: version, MerchantPolicyVersionAtReview: policyVersion, ReviewAfter: command.Input.ReviewAfter, IdempotencyKey: command.Input.IdempotencyKey, ReauthorizationResult: reauth, ReauthorizationReasonCodes: reasons, CreatedAt: command.Now}
	if len(review.ActorMetadata) == 0 || !json.Valid(review.ActorMetadata) {
		review.ActorMetadata = json.RawMessage(`{}`)
	}
	var scheduled *orchestrator.ScheduledAction
	if canSchedule {
		scheduledID := domain.ID(id.New())
		actionID := domain.ID(id.New())
		newPolicyID := domain.ID(id.New())
		scheduledFor := command.Now
		if command.Item.RecommendedAction == domain.ActionRetryLater {
			scheduledFor = command.Now.Add(24 * time.Hour)
		}
		key := "human-approved:" + string(scheduledID)
		parameters, _ := json.Marshal(map[string]any{"decision_id": command.Item.DecisionID, "human_review_id": review.ID})
		checks, _ := json.Marshal(command.FreshPolicy.Checks)
		_, err = tx.Exec(ctx, `INSERT INTO policy_evaluations(id,decision_id,economic_gate_id,case_id,case_version,selected_action,policy_version,result,reason_codes,checks,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,'APPROVE',$8,$9,$10)`, newPolicyID, command.Item.DecisionID, command.Item.EconomicGateID, command.Item.CaseID, version, command.Item.RecommendedAction, command.FreshPolicy.PolicyVersion, append(reasons, "HUMAN_APPROVAL_SATISFIED"), checks, command.Now)
		if err != nil {
			return review, nil, false, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO recovery_actions(id,case_id,action_type,status,parameters,idempotency_key,scheduled_at,created_at,updated_at)VALUES($1,$2,$3,'SCHEDULED',$4,$5,$6,$7,$7)`, actionID, command.Item.CaseID, command.Item.RecommendedAction, parameters, key, scheduledFor, command.Now)
		if err != nil {
			return review, nil, false, err
		}
		for _, transition := range []struct{ from, to domain.CaseState }{{domain.StateEscalated, domain.StateActionPending}, {domain.StateActionPending, domain.StatePolicyReview}, {domain.StatePolicyReview, domain.StateScheduled}} {
			tag, updateErr := tx.Exec(ctx, `UPDATE recovery_cases SET current_state=$2,version=version+1,updated_at=$3 WHERE id=$1 AND current_state=$4`, command.Item.CaseID, transition.to, command.Now, transition.from)
			if updateErr != nil || tag.RowsAffected() != 1 {
				if updateErr != nil {
					return review, nil, false, updateErr
				}
				return review, nil, false, recovery.ErrConflict
			}
			version++
		}
		_, err = tx.Exec(ctx, `INSERT INTO scheduled_actions(id,case_id,decision_id,policy_evaluation_id,recovery_action_id,action,parameters,scheduled_for,status,max_attempts,idempotency_key,case_version_at_schedule,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',3,$9,$10,$11)`, scheduledID, command.Item.CaseID, command.Item.DecisionID, newPolicyID, actionID, command.Item.RecommendedAction, parameters, scheduledFor, key, version, command.Now)
		if err != nil {
			return review, nil, false, err
		}
		review.ScheduledActionID = &scheduledID
		scheduled = &orchestrator.ScheduledAction{ID: scheduledID, CaseID: command.Item.CaseID, DecisionID: command.Item.DecisionID, PolicyEvaluationID: newPolicyID, RecoveryActionID: actionID, Action: command.Item.RecommendedAction, Parameters: parameters, ScheduledFor: scheduledFor, Status: "PENDING", MaxAttempts: 3, IdempotencyKey: key, CaseVersionAtSchedule: version}
	}
	if command.Input.Decision == operations.Stop {
		tag, updateErr := tx.Exec(ctx, `UPDATE recovery_cases SET current_state='STOPPED',version=version+1,updated_at=$2 WHERE id=$1 AND current_state='ESCALATED'`, command.Item.CaseID, command.Now)
		if updateErr != nil || tag.RowsAffected() != 1 {
			if updateErr != nil {
				return review, nil, false, updateErr
			}
			return review, nil, false, recovery.ErrConflict
		}
		version++
	}
	_, err = tx.Exec(ctx, `INSERT INTO human_review_records(`+reviewColumns+`)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, review.ID, review.CaseID, review.DecisionID, review.PolicyEvaluationID, review.RecommendedAction, review.OperatorID, review.ActorType, review.ActorMetadata, review.Decision, review.ReasonCode, review.Notes, review.CaseVersionAtReview, review.MerchantPolicyVersionAtReview, review.ReviewAfter, review.IdempotencyKey, review.ReauthorizationResult, review.ReauthorizationReasonCodes, review.ScheduledActionID, review.CreatedAt)
	if err != nil {
		return review, nil, false, err
	}
	sequence, err := nextSequence(ctx, tx, review.CaseID)
	if err != nil {
		return review, nil, false, err
	}
	eventType := map[operations.ReviewDecision]domain.EventType{operations.Approve: domain.EventOperatorApproved, operations.Reject: domain.EventOperatorRejected, operations.Defer: domain.EventOperatorDeferred, operations.Stop: domain.EventOperatorStopped}[review.Decision]
	if review.ReauthorizationResult == "STALE_APPROVAL" || review.ReauthorizationResult == "DENIED" || review.ReauthorizationResult == "STOPPED" {
		eventType = domain.EventStaleApproval
	}
	payload, _ := json.Marshal(review)
	actor := domain.Actor{Type: review.ActorType, ID: review.OperatorID}
	events := []domain.RecoveryEvent{{ID: domain.ID(id.New()), CaseID: review.CaseID, Sequence: sequence, Type: eventType, Timestamp: command.Now, Actor: actor, Payload: payload, CorrelationID: review.IdempotencyKey}}
	if canSchedule {
		events = append(events, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: review.CaseID, Sequence: sequence + 1, Type: domain.EventPolicyRevalidated, Timestamp: command.Now, Actor: domain.Actor{Type: "POLICY", ID: command.FreshPolicy.PolicyVersion}, Payload: payload, CorrelationID: review.IdempotencyKey}, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: review.CaseID, Sequence: sequence + 2, Type: domain.EventActionScheduled, Timestamp: command.Now, Actor: actor, Payload: payload, CorrelationID: review.IdempotencyKey})
	}
	if command.Input.Decision == operations.Stop {
		events = append(events, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: review.CaseID, Sequence: sequence + 1, Type: domain.EventCaseStopped, Timestamp: command.Now, Actor: actor, Payload: payload, CorrelationID: review.IdempotencyKey})
	}
	for _, event := range events {
		if err = insertEvent(ctx, tx, event); err != nil {
			return review, nil, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return review, nil, false, err
	}
	return review, scheduled, true, nil
}

func (p *Postgres) OperationsMetrics(ctx context.Context) (operations.Metrics, error) {
	var value operations.Metrics
	err := p.pool.QueryRow(ctx, `WITH latest AS(SELECT DISTINCT ON(case_id) case_id,decision,reauthorization_result,created_at FROM human_review_records ORDER BY case_id,created_at DESC,id DESC),pending AS(SELECT c.id,c.amount_at_risk_minor,c.recovery_deadline,pe.created_at FROM recovery_cases c JOIN LATERAL(SELECT created_at FROM policy_evaluations WHERE case_id=c.id AND result='ESCALATE' ORDER BY created_at DESC LIMIT 1)pe ON TRUE LEFT JOIN latest l ON l.case_id=c.id WHERE c.current_state='ESCALATED' AND (l.decision IS NULL OR l.decision='DEFER')) SELECT (SELECT COUNT(*) FROM pending),(SELECT COALESCE(SUM(amount_at_risk_minor),0) FROM pending),(SELECT COUNT(*) FROM human_review_records WHERE decision='APPROVE'),(SELECT COUNT(*) FROM human_review_records WHERE decision='REJECT'),(SELECT COUNT(*) FROM human_review_records WHERE decision='DEFER'),(SELECT COUNT(*) FROM human_review_records WHERE decision='STOP'),(SELECT COUNT(*) FROM human_review_records WHERE reauthorization_result='STALE_APPROVAL'),(SELECT COUNT(*) FROM pending WHERE recovery_deadline<=NOW()),COALESCE((SELECT percentile_cont(.5) WITHIN GROUP(ORDER BY EXTRACT(EPOCH FROM(l.created_at-p.created_at))) FROM latest l JOIN LATERAL(SELECT created_at FROM policy_evaluations WHERE case_id=l.case_id AND result='ESCALATE' ORDER BY created_at DESC LIMIT 1)p ON TRUE),0)`).Scan(&value.PendingReviews, &value.ValueAwaitingReviewMinor, &value.Approvals, &value.Rejections, &value.Deferrals, &value.Stops, &value.StaleApprovals, &value.ExpiredReviews, &value.MedianReviewSeconds)
	return value, err
}
