package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/recovery"
)

func (p *Postgres) SaveDecision(ctx context.Context, s decisioning.Snapshot) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version int64
	var currentState domain.CaseState
	if err = tx.QueryRow(ctx, `SELECT version,current_state FROM recovery_cases WHERE id=$1 FOR UPDATE`, s.Decision.CaseID).Scan(&version, &currentState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return recovery.ErrNotFound
		}
		return err
	}
	if version != s.Decision.CaseVersion {
		return recovery.ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO natural_recovery_predictions(id,case_id,case_version,context_version,probability,model_version,feature_version,predicted_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, s.Natural.ID, s.Natural.CaseID, s.Natural.CaseVersion, s.Natural.ContextVersion, s.Natural.Probability, s.Natural.ModelVersion, s.Natural.FeatureVersion, s.Natural.PredictedAt)
	if err != nil {
		return err
	}
	selected := s.Decision.Optimization.Selected
	profileSnapshot, _ := json.Marshal(s.Decision.Optimization)
	var profileID any
	if s.Decision.Optimization.MerchantProfileID != "" {
		profileID = s.Decision.Optimization.MerchantProfileID
	}
	_, err = tx.Exec(ctx, `INSERT INTO recovery_decisions(id,case_id,case_version,optimizer_version,merchant_objective,context_version,outcome_model_version,natural_model_version,cost_model_version,selected_action,selected_nerv_minor,created_at,merchant_profile_id,merchant_profile_version,merchant_profile_snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, s.Decision.ID, s.Decision.CaseID, s.Decision.CaseVersion, s.Decision.Optimization.OptimizerVersion, s.Decision.Optimization.MerchantObjective, s.Decision.ContextVersion, s.Decision.OutcomeModelVersion, s.Decision.NaturalModelVersion, s.Decision.Optimization.CostModelVersion, selected.Action, selected.NERVMinor, s.Decision.Optimization.CreatedAt, profileID, s.Decision.Optimization.MerchantProfileVersion, profileSnapshot)
	if err != nil {
		return err
	}
	for _, c := range s.Decision.Optimization.Candidates {
		_, err = tx.Exec(ctx, `INSERT INTO recovery_decision_candidates(id,decision_id,action,action_probability,natural_probability,incremental_uplift,gross_incremental_value_minor,channel_cost_minor,incentive_cost_minor,operational_cost_minor,fatigue_penalty_minor,risk_penalty_minor,nerv_minor,objective_score_minor,ranking_position,reason_codes)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id.New(), s.Decision.ID, c.Action, c.ActionRecoveryProbability, c.NaturalRecoveryProbability, c.IncrementalUplift, c.GrossIncrementalValueMinor, c.ChannelCostMinor, c.IncentiveCostMinor, c.OperationalCostMinor, c.FatiguePenaltyMinor, c.RiskPenaltyMinor, c.NERVMinor, c.ObjectiveScoreMinor, c.Rank, c.ReasonCodes)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO economic_gate_evaluations(id,decision_id,case_id,action,nerv_minor,threshold_minor,result,reason_code,gate_version,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s.Gate.ID, s.Gate.DecisionID, s.Gate.CaseID, s.Gate.Action, s.Gate.NERVMinor, s.Gate.ThresholdMinor, s.Gate.Decision, s.Gate.Reason, s.Gate.GateVersion, s.Gate.CreatedAt)
	if err != nil {
		return err
	}
	checks, _ := json.Marshal(s.Policy.Checks)
	_, err = tx.Exec(ctx, `INSERT INTO policy_evaluations(id,decision_id,economic_gate_id,case_id,case_version,selected_action,policy_version,result,reason_codes,checks,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, s.Policy.ID, s.Policy.DecisionID, s.Policy.EconomicGateID, s.Policy.CaseID, s.Policy.CaseVersion, s.Policy.SelectedAction, s.Policy.PolicyVersion, s.Policy.Decision, s.Policy.ReasonCodes, checks, s.Policy.CreatedAt)
	if err != nil {
		return err
	}
	sequence, err := nextSequence(ctx, tx, s.Decision.CaseID)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"decision_id": s.Decision.ID, "case_version": s.Decision.CaseVersion, "selected_action": selected.Action, "nerv_minor": selected.NERVMinor, "natural_probability": s.Natural.Probability})
	policyEvent := map[string]domain.EventType{"APPROVE": domain.EventPolicyApproved, "DENY": domain.EventPolicyDenied, "ESCALATE": domain.EventPolicyEscalated, "STOP": domain.EventCaseStopped}[s.Policy.Decision]
	policyPayload, _ := json.Marshal(map[string]any{"decision_id": s.Decision.ID, "result": s.Policy.Decision, "reason_codes": s.Policy.ReasonCodes, "checks": s.Policy.Checks})
	events := []domain.RecoveryEvent{{ID: domain.ID(id.New()), CaseID: s.Decision.CaseID, Sequence: sequence, Type: domain.EventDecisionCreated, Timestamp: s.Decision.Optimization.CreatedAt, Actor: domain.Actor{Type: "SYSTEM", ID: s.Decision.Optimization.OptimizerVersion}, Payload: payload, ModelVersion: s.Decision.OutcomeModelVersion, CorrelationID: string(s.Decision.ID)}, {ID: domain.ID(id.New()), CaseID: s.Decision.CaseID, Sequence: sequence + 1, Type: map[bool]domain.EventType{true: domain.EventEconomicGateAllowed, false: domain.EventEconomicGateBlocked}[s.Gate.Decision == "ALLOW"], Timestamp: s.Gate.CreatedAt, Actor: domain.Actor{Type: "SYSTEM", ID: s.Gate.GateVersion}, Payload: json.RawMessage(`{}`), CorrelationID: string(s.Decision.ID)}, {ID: domain.ID(id.New()), CaseID: s.Decision.CaseID, Sequence: sequence + 2, Type: domain.EventPolicyEvaluated, Timestamp: s.Policy.CreatedAt, Actor: domain.Actor{Type: "POLICY", ID: s.Policy.PolicyVersion}, Payload: checks, CorrelationID: string(s.Decision.ID)}, {ID: domain.ID(id.New()), CaseID: s.Decision.CaseID, Sequence: sequence + 3, Type: policyEvent, Timestamp: s.Policy.CreatedAt, Actor: domain.Actor{Type: "POLICY", ID: s.Policy.PolicyVersion}, Payload: policyPayload, CorrelationID: string(s.Decision.ID)}}
	if s.Policy.Decision == "ESCALATE" {
		events = append(events, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: s.Decision.CaseID, Sequence: sequence + int64(len(events)), Type: domain.EventHumanReviewRequested, Timestamp: s.Policy.CreatedAt, Actor: domain.Actor{Type: "POLICY", ID: s.Policy.PolicyVersion}, Payload: policyPayload, CorrelationID: string(s.Decision.ID)})
	}
	for _, event := range events {
		if err = insertEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if s.Policy.Decision == "ESCALATE" {
		command, updateErr := tx.Exec(ctx, `UPDATE recovery_cases SET current_state='ESCALATED',version=version+1,updated_at=$2 WHERE id=$1 AND current_state='ACTION_PENDING' AND version=$3`, s.Decision.CaseID, s.Policy.CreatedAt, s.Decision.CaseVersion)
		if updateErr != nil {
			return updateErr
		}
		if command.RowsAffected() != 1 {
			return recovery.ErrConflict
		}
	} else if s.Policy.Decision == "STOP" && !currentState.IsTerminal() {
		command, updateErr := tx.Exec(ctx, `UPDATE recovery_cases SET current_state='STOPPED',version=version+1,updated_at=$2 WHERE id=$1 AND NOT(current_state IN('RECOVERED','EXHAUSTED','STOPPED')) AND version=$3`, s.Decision.CaseID, s.Policy.CreatedAt, s.Decision.CaseVersion)
		if updateErr != nil {
			return updateErr
		}
		if command.RowsAffected() != 1 {
			return recovery.ErrConflict
		}
	}
	return tx.Commit(ctx)
}
