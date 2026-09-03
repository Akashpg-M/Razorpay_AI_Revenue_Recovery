package store

import (
	"context"
	"encoding/json"
	"revenue-recovery/backend/internal/attribution"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/policy"
	"revenue-recovery/backend/internal/replay"
)

func (p *Postgres) rawCaseRows(ctx context.Context, caseID domain.ID, query string) ([]json.RawMessage, error) {
	rows, err := p.pool.Query(ctx, query, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		values = append(values, raw)
	}
	return values, rows.Err()
}
func (p *Postgres) GetReplay(ctx context.Context, caseID domain.ID) (replay.View, error) {
	recoveryCase, err := p.GetCase(ctx, caseID)
	if err != nil {
		return replay.View{}, err
	}
	value := replay.View{Case: recoveryCase, Provenance: map[string]any{"policy_version": policy.Version, "attribution_rule_version": attribution.RuleVersion}}
	err = p.pool.QueryRow(ctx, `SELECT row_to_json(m)::jsonb FROM merchants m WHERE id=$1`, recoveryCase.MerchantID).Scan(&value.Merchant)
	if err != nil {
		return value, err
	}
	queries := []struct {
		target *[]json.RawMessage
		sql    string
	}{
		{&value.Events, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM recovery_events WHERE case_id=$1 ORDER BY sequence) x`},
		{&value.Decisions, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM recovery_decisions WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.Candidates, `SELECT row_to_json(x)::jsonb FROM (SELECT c.* FROM recovery_decision_candidates c JOIN recovery_decisions d ON d.id=c.decision_id WHERE d.case_id=$1 ORDER BY d.created_at,c.ranking_position) x`},
		{&value.EconomicGates, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM economic_gate_evaluations WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.PolicyEvaluations, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM policy_evaluations WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.Actions, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM recovery_actions WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.Schedules, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM scheduled_actions WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.Executions, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM executions WHERE case_id=$1 ORDER BY started_at,id) x`},
		{&value.Promises, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM promises_to_pay WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.PromiseEvents, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM promise_events WHERE case_id=$1 ORDER BY occurred_at,id) x`},
		{&value.PromiseChecks, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM promise_checks WHERE case_id=$1 ORDER BY scheduled_for,id) x`},
		{&value.HumanReviews, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM human_review_records WHERE case_id=$1 ORDER BY created_at,id) x`},
		{&value.Attributions, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM recovery_attributions WHERE case_id=$1 ORDER BY observed_at,id) x`},
		{&value.ProviderReferences, `SELECT row_to_json(x)::jsonb FROM (SELECT r.* FROM provider_action_references r JOIN recovery_actions a ON a.id=r.action_id WHERE a.case_id=$1 ORDER BY r.created_at,r.id) x`},
		{&value.WebhookEvents, `SELECT row_to_json(x)::jsonb FROM (SELECT w.* FROM webhook_events w WHERE w.provider_references->>'payment_link_id' IN (SELECT r.provider_reference FROM provider_action_references r JOIN recovery_actions a ON a.id=r.action_id WHERE a.case_id=$1) OR w.provider_references->>'payment_id'=(SELECT source_reference FROM recovery_cases WHERE id=$1) ORDER BY w.received_at,w.id) x`},
		{&value.FeedbackRecords, `SELECT row_to_json(x)::jsonb FROM (SELECT * FROM feedback_records WHERE case_id=$1 ORDER BY created_at,id) x`},
	}
	for _, query := range queries {
		*query.target, err = p.rawCaseRows(ctx, caseID, query.sql)
		if err != nil {
			return value, err
		}
	}
	if len(value.Decisions) > 0 {
		var latest map[string]any
		if json.Unmarshal(value.Decisions[len(value.Decisions)-1], &latest) == nil {
			for _, key := range []string{"context_version", "outcome_model_version", "natural_model_version", "optimizer_version", "cost_model_version", "merchant_profile_version"} {
				if item, ok := latest[key]; ok {
					value.Provenance[key] = item
				}
			}
		}
	}
	if len(value.EconomicGates) > 0 {
		var latest map[string]any
		if json.Unmarshal(value.EconomicGates[len(value.EconomicGates)-1], &latest) == nil {
			if version, ok := latest["gate_version"]; ok {
				value.Provenance["economic_gate_version"] = version
			}
		}
	}
	return value, nil
}
