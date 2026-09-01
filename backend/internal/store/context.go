package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/domain"
)

func (p *Postgres) LoadContextInputs(ctx context.Context, c domain.RecoveryCase) (domain.Customer, domain.CustomerRecoveryProfile, domain.Merchant, domain.MerchantPolicy, []domain.RecoveryAction, *domain.PromiseToPay, error) {
	var customer domain.Customer
	err := p.pool.QueryRow(ctx, `SELECT id,merchant_id,external_id,contact,opted_out,created_at,updated_at FROM customers WHERE id=$1`, c.CustomerID).
		Scan(&customer.ID, &customer.MerchantID, &customer.ExternalID, &customer.Contact, &customer.OptedOut, &customer.CreatedAt, &customer.UpdatedAt)
	if err != nil {
		return customer, domain.CustomerRecoveryProfile{}, domain.Merchant{}, domain.MerchantPolicy{}, nil, nil, err
	}
	var profile domain.CustomerRecoveryProfile
	err = p.pool.QueryRow(ctx, `SELECT id,customer_id,successful_payments,failed_payments,subscription_tenure_days,promise_reliability,fatigue_score,features,version,updated_at FROM customer_recovery_profiles WHERE customer_id=$1 ORDER BY version DESC LIMIT 1`, c.CustomerID).
		Scan(&profile.ID, &profile.CustomerID, &profile.SuccessfulPayments, &profile.FailedPayments, &profile.SubscriptionTenureDays, &profile.PromiseReliability, &profile.FatigueScore, &profile.Features, &profile.Version, &profile.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		profile = domain.CustomerRecoveryProfile{CustomerID: c.CustomerID, PromiseReliability: .5, Features: json.RawMessage(`{}`)}
	} else if err != nil {
		return customer, profile, domain.Merchant{}, domain.MerchantPolicy{}, nil, nil, err
	}
	var merchant domain.Merchant
	err = p.pool.QueryRow(ctx, `SELECT id,name,merchant_type,metadata,created_at,updated_at FROM merchants WHERE id=$1`, c.MerchantID).Scan(&merchant.ID, &merchant.Name, &merchant.Type, &merchant.Metadata, &merchant.CreatedAt, &merchant.UpdatedAt)
	if err != nil {
		return customer, profile, merchant, domain.MerchantPolicy{}, nil, nil, err
	}
	var policy domain.MerchantPolicy
	var allowedActions []string
	err = p.pool.QueryRow(ctx, `SELECT id,merchant_id,objective,max_retries,max_contacts_per_day,max_contacts_per_week,min_contact_interval_minutes,recovery_window_hours,quiet_hours,allowed_actions,allowed_channels,high_value_threshold_minor,low_confidence_threshold,minimum_economic_value_minor,maximum_incentive_minor,requires_high_value_human_approval,version,created_at,updated_at FROM merchant_policies WHERE merchant_id=$1 ORDER BY version DESC LIMIT 1`, c.MerchantID).
		Scan(&policy.ID, &policy.MerchantID, &policy.Objective, &policy.MaxRetries, &policy.MaxContactsPerDay, &policy.MaxContactsPerWeek, &policy.MinContactIntervalMinutes, &policy.RecoveryWindowHours, &policy.QuietHours, &allowedActions, &policy.AllowedChannels, &policy.HighValueThresholdMinor, &policy.LowConfidenceThreshold, &policy.MinimumEconomicValueMinor, &policy.MaximumIncentiveMinor, &policy.RequiresHighValueHumanApproval, &policy.Version, &policy.CreatedAt, &policy.UpdatedAt)
	if err != nil {
		return customer, profile, merchant, policy, nil, nil, err
	}
	policy.AllowedActions = make([]domain.ActionType, len(allowedActions))
	for index, value := range allowedActions {
		policy.AllowedActions[index] = domain.ActionType(value)
	}
	var optimization domain.MerchantOptimizationProfile
	var profileActions []string
	err = p.pool.QueryRow(ctx, `SELECT id,merchant_id,objective,revenue_weight_bps,retention_weight_bps,contact_penalty_weight_bps,cost_penalty_weight_bps,fatigue_penalty_weight_bps,risk_penalty_weight_bps,escalation_preference,allowed_actions,allowed_channels,minimum_nerv_minor,discount_budget_minor,human_review_budget_minor,configuration_version,created_at FROM merchant_optimization_profiles WHERE merchant_id=$1 ORDER BY configuration_version DESC LIMIT 1`, c.MerchantID).Scan(&optimization.ID, &optimization.MerchantID, &optimization.Objective, &optimization.RevenueWeightBPS, &optimization.RetentionWeightBPS, &optimization.ContactPenaltyWeightBPS, &optimization.CostPenaltyWeightBPS, &optimization.FatiguePenaltyWeightBPS, &optimization.RiskPenaltyWeightBPS, &optimization.EscalationPreference, &profileActions, &optimization.AllowedChannels, &optimization.MinimumNERVMinor, &optimization.DiscountBudgetMinor, &optimization.HumanReviewBudgetMinor, &optimization.ConfigurationVersion, &optimization.CreatedAt)
	if err == nil {
		optimization.AllowedActions = make([]domain.ActionType, len(profileActions))
		for i, value := range profileActions {
			optimization.AllowedActions[i] = domain.ActionType(value)
		}
		policy.OptimizationProfile = &optimization
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return customer, profile, merchant, policy, nil, nil, err
	}
	// The currently scheduled action is authorization input, not historical
	// customer contact/retry activity. Counting it here would self-trigger its
	// own cooldown during the worker's fresh policy check.
	rows, err := p.pool.Query(ctx, `SELECT id,case_id,action_type,status,parameters,idempotency_key,policy_decision_id,scheduled_at,created_at,updated_at FROM recovery_actions WHERE case_id=$1 AND status<>'SCHEDULED' ORDER BY created_at DESC LIMIT 50`, c.ID)
	if err != nil {
		return customer, profile, merchant, policy, nil, nil, err
	}
	defer rows.Close()
	actions := []domain.RecoveryAction{}
	for rows.Next() {
		var a domain.RecoveryAction
		if err = rows.Scan(&a.ID, &a.CaseID, &a.Type, &a.Status, &a.Parameters, &a.IdempotencyKey, &a.PolicyDecisionID, &a.ScheduledAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return customer, profile, merchant, policy, nil, nil, err
		}
		actions = append(actions, a)
	}
	var promise domain.PromiseToPay
	err = p.pool.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE case_id=$1 AND status='ACTIVE' ORDER BY created_at DESC LIMIT 1`, c.ID).Scan(&promise.ID, &promise.CaseID, &promise.CustomerID, &promise.Status, &promise.DueAt, &promise.Confidence, &promise.Source, &promise.CreatedAt, &promise.ResolvedAt, &promise.PromisedAmountMinor, &promise.ExtractorVersion, &promise.ExtractionTimestamp, &promise.SourceResponseID, &promise.FulfilledAt, &promise.BrokenAt, &promise.ExpiredAt, &promise.CancelledAt, &promise.VerificationReference)
	var active *domain.PromiseToPay
	if err == nil {
		active = &promise
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return customer, profile, merchant, policy, nil, nil, err
	}
	return customer, profile, merchant, policy, actions, active, rows.Err()
}
