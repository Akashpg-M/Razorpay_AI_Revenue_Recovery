package merchantprofile

import (
	"context"
	"errors"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
)

type Store interface {
	GetMerchantOptimizationProfile(context.Context, domain.ID) (domain.MerchantOptimizationProfile, error)
	CreateMerchantOptimizationProfile(context.Context, domain.MerchantOptimizationProfile) (domain.MerchantOptimizationProfile, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Get(ctx context.Context, merchantID domain.ID) (domain.MerchantOptimizationProfile, error) {
	return s.store.GetMerchantOptimizationProfile(ctx, merchantID)
}

func (s *Service) Create(ctx context.Context, profile domain.MerchantOptimizationProfile) (domain.MerchantOptimizationProfile, error) {
	if profile.MerchantID == "" {
		return profile, errors.New("merchant_id is required")
	}
	validObjectives := map[string]bool{"MAXIMIZE_NET_RECOVERY": true, "MAXIMIZE_RETENTION": true, "MINIMIZE_CONTACT": true, "MINIMIZE_RECOVERY_COST": true, "BALANCED": true}
	if !validObjectives[profile.Objective] {
		return profile, errors.New("invalid optimization objective")
	}
	weights := []int64{profile.RevenueWeightBPS, profile.RetentionWeightBPS, profile.ContactPenaltyWeightBPS, profile.CostPenaltyWeightBPS, profile.FatiguePenaltyWeightBPS, profile.RiskPenaltyWeightBPS}
	for _, weight := range weights {
		if weight < 0 || weight > 20000 {
			return profile, errors.New("profile weights must be between 0 and 20000 bps")
		}
	}
	if profile.RevenueWeightBPS+profile.RetentionWeightBPS == 0 {
		return profile, errors.New("at least one positive value weight is required")
	}
	if profile.EscalationPreference == "" {
		profile.EscalationPreference = "STANDARD"
	}
	if profile.EscalationPreference != "STANDARD" && profile.EscalationPreference != "CONSERVATIVE" && profile.EscalationPreference != "AGGRESSIVE" {
		return profile, errors.New("invalid escalation preference")
	}
	if profile.MinimumNERVMinor < 0 || profile.DiscountBudgetMinor < 0 || profile.HumanReviewBudgetMinor < 0 {
		return profile, errors.New("budgets and minimum NERV must be non-negative")
	}
	if profile.AllowedActions == nil {
		profile.AllowedActions = []domain.ActionType{}
	}
	if profile.AllowedChannels == nil {
		profile.AllowedChannels = []string{}
	}
	profile.ID = domain.ID(id.New())
	profile.CreatedAt = s.now().UTC()
	return s.store.CreateMerchantOptimizationProfile(ctx, profile)
}
