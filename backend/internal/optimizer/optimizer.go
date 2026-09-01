package optimizer

import (
	"math"
	"math/big"
	"sort"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
)

const OptimizerVersion = "nba-v2-profiled"
const CostModelVersion = "cost-v1"

type Cost struct {
	ChannelMinor     int64
	OperationalMinor int64
	Contact          bool
}
type CostModel struct {
	Version               string
	Actions               map[domain.ActionType]Cost
	FatiguePenaltyBPS     int64
	RiskPenaltyBPS        int64
	RetentionIncentiveBPS int64
}

func DefaultCostModel() CostModel {
	return CostModel{Version: CostModelVersion, FatiguePenaltyBPS: 80, RiskPenaltyBPS: 50, RetentionIncentiveBPS: 500, Actions: map[domain.ActionType]Cost{
		domain.ActionWait: {}, domain.ActionRetryNow: {OperationalMinor: 35}, domain.ActionRetryLater: {OperationalMinor: 35},
		domain.ActionSendReminder: {ChannelMinor: 25, Contact: true}, domain.ActionSendPaymentLink: {ChannelMinor: 25, OperationalMinor: 45, Contact: true},
		domain.ActionSendCheckoutRecoveryLink: {ChannelMinor: 25, OperationalMinor: 45, Contact: true}, domain.ActionRequestPaymentMethodUpdate: {ChannelMinor: 25, OperationalMinor: 35, Contact: true},
		domain.ActionSuggestAlternateMethod: {ChannelMinor: 25, OperationalMinor: 30, Contact: true}, domain.ActionWaitForPromiseToPay: {OperationalMinor: 10},
		domain.ActionRetention: {ChannelMinor: 25, OperationalMinor: 100, Contact: true},
	}}
}

type Candidate struct {
	Action                     domain.ActionType `json:"action"`
	ActionRecoveryProbability  float64           `json:"action_recovery_probability"`
	NaturalRecoveryProbability float64           `json:"natural_recovery_probability"`
	IncrementalUplift          float64           `json:"incremental_uplift"`
	GrossIncrementalValueMinor int64             `json:"gross_incremental_recovery_value_minor"`
	ChannelCostMinor           int64             `json:"channel_cost_minor"`
	IncentiveCostMinor         int64             `json:"incentive_cost_minor"`
	OperationalCostMinor       int64             `json:"operational_cost_minor"`
	FatiguePenaltyMinor        int64             `json:"fatigue_penalty_minor"`
	RiskPenaltyMinor           int64             `json:"risk_penalty_minor"`
	NERVMinor                  int64             `json:"net_expected_recovery_value_minor"`
	ObjectiveScoreMinor        int64             `json:"objective_score_minor"`
	Rank                       int               `json:"ranking_position"`
	ReasonCodes                []string          `json:"reason_codes"`
}
type Result struct {
	OptimizerVersion       string      `json:"optimizer_version"`
	CostModelVersion       string      `json:"cost_model_version"`
	MerchantObjective      string      `json:"merchant_objective"`
	Candidates             []Candidate `json:"candidates"`
	Selected               Candidate   `json:"selected_candidate"`
	CreatedAt              time.Time   `json:"created_at"`
	MerchantProfileID      domain.ID   `json:"merchant_profile_id,omitempty"`
	MerchantProfileVersion int         `json:"merchant_profile_version"`
}

func Rank(ctx recoverycontext.RecoveryDecisionContext, predictions []decisionclient.OutcomePrediction, natural float64, costs CostModel, now time.Time) Result {
	byAction := map[domain.ActionType]float64{}
	for _, prediction := range predictions {
		byAction[domain.ActionType(prediction.Action)] = prediction.RecoveryProbability
	}
	byAction[domain.ActionWait] = natural
	candidates := make([]Candidate, 0, len(byAction))
	for action, probability := range byAction {
		uplift := probability - natural
		gross := probabilityValue(ctx.Case.AmountAtRiskMinor, uplift)
		cost := costs.Actions[action]
		fatigue := int64(0)
		if cost.Contact {
			fatigue = mulDiv(ctx.Case.AmountAtRiskMinor, int64(math.Round(ctx.CustomerProfile.RecoveryFatigue*float64(costs.FatiguePenaltyBPS))), 10000)
		}
		risk := mulDiv(ctx.Case.AmountAtRiskMinor, int64(math.Round((1-ctx.Diagnosis.Confidence)*float64(costs.RiskPenaltyBPS))), 10000)
		if action == domain.ActionWait {
			risk = 0
		}
		incentive := int64(0)
		if action == domain.ActionRetention {
			incentive = mulDiv(ctx.Case.AmountAtRiskMinor, costs.RetentionIncentiveBPS, 10000)
			if max := ctx.MerchantContext.MaximumIncentiveMinor; max > 0 && incentive > max {
				incentive = max
			}
		}
		nerv := gross - cost.ChannelMinor - cost.OperationalMinor - incentive - fatigue - risk
		score := objectiveScore(ctx.MerchantContext, ctx.Case.AmountAtRiskMinor, gross, nerv, cost.ChannelMinor+cost.OperationalMinor+incentive, fatigue, risk)
		reasons := []string{"INCREMENTAL_VALUE_SCORING"}
		if uplift < 0 {
			reasons = append(reasons, "NEGATIVE_INCREMENTAL_UPLIFT")
		}
		if action == domain.ActionWait {
			reasons = append(reasons, "NATURAL_RECOVERY_REFERENCE")
		}
		if nerv < 0 {
			reasons = append(reasons, "NEGATIVE_NERV")
		}
		candidates = append(candidates, Candidate{Action: action, ActionRecoveryProbability: probability, NaturalRecoveryProbability: natural, IncrementalUplift: uplift, GrossIncrementalValueMinor: gross, ChannelCostMinor: cost.ChannelMinor, IncentiveCostMinor: incentive, OperationalCostMinor: cost.OperationalMinor, FatiguePenaltyMinor: fatigue, RiskPenaltyMinor: risk, NERVMinor: nerv, ObjectiveScoreMinor: score, ReasonCodes: reasons})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ObjectiveScoreMinor != candidates[j].ObjectiveScoreMinor {
			return candidates[i].ObjectiveScoreMinor > candidates[j].ObjectiveScoreMinor
		}
		if candidates[i].NERVMinor != candidates[j].NERVMinor {
			return candidates[i].NERVMinor > candidates[j].NERVMinor
		}
		return candidates[i].Action < candidates[j].Action
	})
	for index := range candidates {
		candidates[index].Rank = index + 1
	}
	return Result{OptimizerVersion: OptimizerVersion, CostModelVersion: costs.Version, MerchantObjective: ctx.MerchantContext.RecoveryObjective, Candidates: candidates, Selected: candidates[0], CreatedAt: now.UTC(), MerchantProfileID: ctx.MerchantContext.OptimizationProfileID, MerchantProfileVersion: ctx.MerchantContext.OptimizationProfileVersion}
}

func probabilityValue(amount int64, probability float64) int64 {
	return mulDiv(amount, int64(math.Round(probability*1_000_000)), 1_000_000)
}
func mulDiv(a, b, divisor int64) int64 {
	value := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	value.Quo(value, big.NewInt(divisor))
	return value.Int64()
}
func objectiveScore(ctx recoverycontext.MerchantContext, amountAtRisk, gross, nerv, cost, fatigue, risk int64) int64 {
	if ctx.OptimizationProfileVersion > 0 {
		retention := gross - risk
		contact := int64(0)
		if fatigue > 0 {
			contact = amountAtRisk
		}
		return mulDiv(gross, ctx.RevenueWeightBPS, 10000) + mulDiv(retention, ctx.RetentionWeightBPS, 10000) - mulDiv(contact, ctx.ContactPenaltyWeightBPS, 10000) - mulDiv(cost, ctx.CostPenaltyWeightBPS, 10000) - mulDiv(fatigue, ctx.FatiguePenaltyWeightBPS, 10000) - mulDiv(risk, ctx.RiskPenaltyWeightBPS, 10000)
	}
	switch ctx.RecoveryObjective {
	case "MINIMIZE_CONTACT":
		return nerv - fatigue*2
	case "MINIMIZE_RECOVERY_COST":
		return nerv - cost
	case "MAXIMIZE_RETENTION":
		return nerv - risk
	case "BALANCED":
		return nerv - (fatigue+risk)/2
	default:
		return nerv
	}
}
