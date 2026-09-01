package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/recovery"
)

func scanMerchantProfile(row rowScanner) (domain.MerchantOptimizationProfile, error) {
	var result domain.MerchantOptimizationProfile
	var actions []string
	err := row.Scan(&result.ID, &result.MerchantID, &result.Objective, &result.RevenueWeightBPS, &result.RetentionWeightBPS, &result.ContactPenaltyWeightBPS, &result.CostPenaltyWeightBPS, &result.FatiguePenaltyWeightBPS, &result.RiskPenaltyWeightBPS, &result.EscalationPreference, &actions, &result.AllowedChannels, &result.MinimumNERVMinor, &result.DiscountBudgetMinor, &result.HumanReviewBudgetMinor, &result.ConfigurationVersion, &result.CreatedAt)
	for _, action := range actions {
		result.AllowedActions = append(result.AllowedActions, domain.ActionType(action))
	}
	return result, err
}

const merchantProfileColumns = `id,merchant_id,objective,revenue_weight_bps,retention_weight_bps,contact_penalty_weight_bps,cost_penalty_weight_bps,fatigue_penalty_weight_bps,risk_penalty_weight_bps,escalation_preference,allowed_actions,allowed_channels,minimum_nerv_minor,discount_budget_minor,human_review_budget_minor,configuration_version,created_at`

func (p *Postgres) GetMerchantOptimizationProfile(ctx context.Context, merchantID domain.ID) (domain.MerchantOptimizationProfile, error) {
	result, err := scanMerchantProfile(p.pool.QueryRow(ctx, `SELECT `+merchantProfileColumns+` FROM merchant_optimization_profiles WHERE merchant_id=$1 ORDER BY configuration_version DESC LIMIT 1`, merchantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, recovery.ErrNotFound
	}
	return result, err
}

func (p *Postgres) CreateMerchantOptimizationProfile(ctx context.Context, profile domain.MerchantOptimizationProfile) (domain.MerchantOptimizationProfile, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return profile, err
	}
	defer tx.Rollback(ctx)
	var locked domain.ID
	if err = tx.QueryRow(ctx, `SELECT id FROM merchants WHERE id=$1 FOR UPDATE`, profile.MerchantID).Scan(&locked); err != nil {
		return profile, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(configuration_version),0)+1 FROM merchant_optimization_profiles WHERE merchant_id=$1`, profile.MerchantID).Scan(&profile.ConfigurationVersion); err != nil {
		return profile, err
	}
	actions := make([]string, len(profile.AllowedActions))
	for i, value := range profile.AllowedActions {
		actions[i] = string(value)
	}
	_, err = tx.Exec(ctx, `INSERT INTO merchant_optimization_profiles(`+merchantProfileColumns+`)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, profile.ID, profile.MerchantID, profile.Objective, profile.RevenueWeightBPS, profile.RetentionWeightBPS, profile.ContactPenaltyWeightBPS, profile.CostPenaltyWeightBPS, profile.FatiguePenaltyWeightBPS, profile.RiskPenaltyWeightBPS, profile.EscalationPreference, actions, profile.AllowedChannels, profile.MinimumNERVMinor, profile.DiscountBudgetMinor, profile.HumanReviewBudgetMinor, profile.ConfigurationVersion, profile.CreatedAt)
	if err != nil {
		return profile, err
	}
	if err = tx.Commit(ctx); err != nil {
		return profile, err
	}
	return profile, nil
}
