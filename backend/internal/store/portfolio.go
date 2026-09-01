package store

import (
	"context"
	"encoding/json"
	"revenue-recovery/backend/internal/budget"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/portfolio"
	"time"
)

func (p *Postgres) LoadPortfolioCandidates(ctx context.Context, merchantID domain.ID) ([]portfolio.Candidate, error) {
	rows, err := p.pool.Query(ctx, `SELECT DISTINCT ON(c.id) c.merchant_id,c.id,d.id,dc.action,c.amount_at_risk_minor,dc.gross_incremental_value_minor,dc.nerv_minor,c.recovery_deadline,c.created_at,ROUND(dc.action_probability*10000)::bigint FROM recovery_cases c JOIN recovery_decisions d ON d.case_id=c.id JOIN recovery_decision_candidates dc ON dc.decision_id=d.id AND dc.ranking_position=1 WHERE c.merchant_id=$1 AND c.current_state IN('ACTION_PENDING','POLICY_REVIEW','SCHEDULED','WAITING_OUTCOME','REASSESSING') ORDER BY c.id,d.created_at DESC`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []portfolio.Candidate{}
	for rows.Next() {
		var item portfolio.Candidate
		if err = rows.Scan(&item.MerchantID, &item.CaseID, &item.DecisionID, &item.Action, &item.AmountAtRiskMinor, &item.ExpectedIncrementalValueMinor, &item.ExpectedNERVMinor, &item.Deadline, &item.ArrivalAt, &item.RecoverabilityBPS); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (p *Postgres) SavePortfolioPriority(ctx context.Context, items []portfolio.Item) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, item := range items {
		_, err = tx.Exec(ctx, `INSERT INTO portfolio_priority_snapshots(id,run_id,merchant_id,case_id,decision_id,action,amount_at_risk_minor,expected_incremental_value_minor,expected_nerv_minor,urgency_bps,recoverability_bps,priority_score_minor,rank,explanation,algorithm_version,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, item.ID, item.RunID, item.MerchantID, item.CaseID, item.DecisionID, item.Action, item.AmountAtRiskMinor, item.ExpectedIncrementalValueMinor, item.ExpectedNERVMinor, item.UrgencyBPS, item.RecoverabilityBPS, item.PriorityScoreMinor, item.Rank, item.Explanation, item.AlgorithmVersion, item.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) LoadPriorityRun(ctx context.Context, runID string) ([]portfolio.Item, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,run_id,merchant_id,case_id,decision_id,action,amount_at_risk_minor,expected_incremental_value_minor,expected_nerv_minor,urgency_bps,recoverability_bps,priority_score_minor,rank,explanation,algorithm_version,created_at FROM portfolio_priority_snapshots WHERE run_id=$1 ORDER BY rank`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []portfolio.Item{}
	for rows.Next() {
		var item portfolio.Item
		if err = rows.Scan(&item.ID, &item.RunID, &item.MerchantID, &item.CaseID, &item.DecisionID, &item.Action, &item.AmountAtRiskMinor, &item.ExpectedIncrementalValueMinor, &item.ExpectedNERVMinor, &item.UrgencyBPS, &item.RecoverabilityBPS, &item.PriorityScoreMinor, &item.Rank, &item.Explanation, &item.AlgorithmVersion, &item.CreatedAt); err != nil {
			return nil, err
		}
		var explanation struct {
			CaseCreatedAt time.Time `json:"case_created_at"`
		}
		if json.Unmarshal(item.Explanation, &explanation) == nil {
			item.ArrivalAt = explanation.CaseCreatedAt
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (p *Postgres) SaveBudgetRun(ctx context.Context, run budget.Run) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	limits, _ := json.Marshal(run.Budget)
	totals, _ := json.Marshal(run.Totals)
	_, err = tx.Exec(ctx, `INSERT INTO budget_allocation_runs(id,merchant_id,algorithm_version,priority_run_id,budget,totals,created_at)VALUES($1,$2,$3,$4,$5,$6,$7)`, run.ID, run.MerchantID, run.AlgorithmVersion, run.PriorityRunID, limits, totals, run.CreatedAt)
	if err != nil {
		return err
	}
	for _, value := range run.Allocations {
		_, err = tx.Exec(ctx, `INSERT INTO budget_allocations(id,run_id,case_id,decision_id,action,expected_incremental_value_minor,expected_nerv_minor,expected_cost_minor,resource_consumption,allocation_rank,included,exclusion_reason,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.ID, value.RunID, value.CaseID, value.DecisionID, value.Action, value.ExpectedIncrementalValueMinor, value.ExpectedNERVMinor, value.ExpectedCostMinor, value.ResourceConsumption, value.Rank, value.Included, value.ExclusionReason, value.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
