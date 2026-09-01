package recoverycontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"revenue-recovery/backend/internal/domain"
)

const FeatureVersion = "recovery-context-v1"

type Diagnosis struct {
	FailureCategory string   `json:"failure_category"`
	Recoverability  string   `json:"recoverability"`
	Confidence      float64  `json:"confidence"`
	Evidence        []string `json:"evidence"`
}

type CustomerProfile struct {
	SuccessfulPaymentRatio       float64 `json:"successful_payment_ratio"`
	SuccessfulPaymentCount       int     `json:"successful_payment_count"`
	RecentFailures               int     `json:"recent_failures"`
	PreviousRecoveryAttempts     int     `json:"previous_recovery_attempts"`
	AverageTransactionValueMinor int64   `json:"average_transaction_value_minor"`
	ContactCount                 int     `json:"contact_count"`
	PromiseReliability           float64 `json:"promise_reliability"`
	RecoveryFatigue              float64 `json:"recovery_fatigue"`
	SubscriptionTenureDays       int     `json:"subscription_tenure_days"`
	PreferredPaymentMethod       string  `json:"preferred_payment_method"`
	PriorCheckoutRecoveries      int     `json:"prior_checkout_recoveries"`
	OptedOut                     bool    `json:"opted_out"`
}

type MerchantContext struct {
	OptimizationProfileID         domain.ID           `json:"optimization_profile_id,omitempty"`
	OptimizationProfileVersion    int                 `json:"optimization_profile_version"`
	MerchantType                  string              `json:"merchant_type"`
	RecoveryObjective             string              `json:"recovery_objective"`
	AllowedChannels               []string            `json:"allowed_channels"`
	AllowedActions                []domain.ActionType `json:"allowed_actions"`
	MaxRetries                    int                 `json:"maximum_retries"`
	MaxContactsPerDay             int                 `json:"maximum_contacts_per_day"`
	MaxContactsPerWeek            int                 `json:"maximum_contacts_per_week"`
	MinimumContactIntervalMinutes int                 `json:"minimum_contact_interval_minutes"`
	RecoveryWindowHours           int                 `json:"recovery_window_hours"`
	QuietHours                    json.RawMessage     `json:"quiet_hours"`
	MinimumEconomicValueMinor     int64               `json:"minimum_net_intervention_value_minor"`
	HighValueThresholdMinor       int64               `json:"high_value_threshold_minor"`
	LowConfidenceThreshold        float64             `json:"low_confidence_escalation_threshold"`
	MaximumIncentiveMinor         int64               `json:"maximum_incentive_minor"`
	Timezone                      string              `json:"timezone"`
	RevenueWeightBPS              int64               `json:"revenue_weight_bps"`
	RetentionWeightBPS            int64               `json:"retention_weight_bps"`
	ContactPenaltyWeightBPS       int64               `json:"contact_penalty_weight_bps"`
	CostPenaltyWeightBPS          int64               `json:"cost_penalty_weight_bps"`
	FatiguePenaltyWeightBPS       int64               `json:"fatigue_penalty_weight_bps"`
	RiskPenaltyWeightBPS          int64               `json:"risk_penalty_weight_bps"`
	EscalationPreference          string              `json:"escalation_preference"`
}

type RecentAction struct {
	Type      domain.ActionType `json:"action"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
}

type RecoveryDecisionContext struct {
	FeatureVersion  string               `json:"feature_version"`
	Case            domain.RecoveryCase  `json:"case"`
	Diagnosis       Diagnosis            `json:"diagnosis"`
	CustomerProfile CustomerProfile      `json:"customer_profile"`
	MerchantContext MerchantContext      `json:"merchant_context"`
	RecentActions   []RecentAction       `json:"recent_actions"`
	ActivePromise   *domain.PromiseToPay `json:"active_promise,omitempty"`
	TimingContext   TimingContext        `json:"timing_context"`
	PaymentState    PaymentState         `json:"payment_state"`
}

type TimingContext struct {
	EvaluatedAt        time.Time `json:"evaluated_at"`
	HoursSinceLeak     float64   `json:"hours_since_leak"`
	HoursUntilDeadline float64   `json:"hours_until_deadline"`
}
type PaymentState struct {
	AlreadyRecovered    bool     `json:"already_recovered"`
	MandateStatus       string   `json:"mandate_status"`
	PaymentMethodStatus string   `json:"payment_method_status"`
	AvailableChannels   []string `json:"available_channels"`
}

type BuildInput struct {
	Case          domain.RecoveryCase
	Customer      domain.Customer
	Profile       domain.CustomerRecoveryProfile
	Merchant      domain.Merchant
	Policy        domain.MerchantPolicy
	Actions       []domain.RecoveryAction
	ActivePromise *domain.PromiseToPay
	Now           time.Time
}

func Diagnose(c domain.RecoveryCase) Diagnosis {
	var raw map[string]any
	_ = json.Unmarshal(c.FailureOrLeakContext, &raw)
	category, _ := raw["failure_category"].(string)
	if category == "" {
		category = "CUSTOMER_INTENT_OR_UNKNOWN"
		if c.LeakType == domain.CheckoutAbandonment {
			category = "UNKNOWN_ABANDONMENT"
		}
	}
	switch category {
	case "AMBIGUOUS_CUSTOMER_INTENT":
		category = "CUSTOMER_INTENT_OR_UNKNOWN"
	case "METHOD_MISMATCH":
		category = "PAYMENT_METHOD_MISMATCH"
	case "TEMPORARY_ABANDONMENT":
		category = "DELAYED_INTENT"
	case "PRICE_HESITATION":
		category = "PRICE_OR_VALUE_HESITATION"
	case "LOW_INTENT_ABANDONMENT":
		category = "UNKNOWN_ABANDONMENT"
	}
	recoverability := "UNKNOWN"
	confidence := 0.55
	switch category {
	case "TEMPORARY_BANK_FAILURE", "INSUFFICIENT_FUNDS", "PAYMENT_FAILURE", "PAYMENT_FRICTION", "DELAYED_INTENT":
		recoverability = "TEMPORARY"
		confidence = 0.9
	case "PAYMENT_METHOD_INVALID", "MANDATE_FAILURE", "PAYMENT_METHOD_MISMATCH", "PRICE_OR_VALUE_HESITATION":
		recoverability = "PERSISTENT"
		confidence = 0.86
	case "HARD_DECLINE":
		recoverability = "NON_RECOVERABLE"
		confidence = 0.94
	case "CUSTOMER_INTENT_OR_UNKNOWN", "UNKNOWN_ABANDONMENT":
		recoverability = "UNKNOWN"
		confidence = 0.5
	default:
		category = "CUSTOMER_INTENT_OR_UNKNOWN"
		recoverability = "UNKNOWN"
		confidence = 0.4
	}
	return Diagnosis{FailureCategory: category, Recoverability: recoverability, Confidence: confidence, Evidence: []string{"normalized_failure_context"}}
}

func Build(input BuildInput) (RecoveryDecisionContext, error) {
	if input.Now.IsZero() {
		return RecoveryDecisionContext{}, errors.New("context evaluation time is required")
	}
	for _, raw := range []json.RawMessage{input.Profile.Features, input.Case.CustomerContextSnapshot, input.Case.FailureOrLeakContext} {
		if err := RejectHiddenFeatures(raw); err != nil {
			return RecoveryDecisionContext{}, err
		}
	}
	diagnosis := Diagnose(input.Case)
	total := input.Profile.SuccessfulPayments + input.Profile.FailedPayments
	features := map[string]any{}
	_ = json.Unmarshal(input.Profile.Features, &features)
	profile := CustomerProfile{SuccessfulPaymentCount: input.Profile.SuccessfulPayments, RecentFailures: input.Profile.FailedPayments,
		PromiseReliability: clamp(input.Profile.PromiseReliability), RecoveryFatigue: clamp(input.Profile.FatigueScore),
		SubscriptionTenureDays: input.Profile.SubscriptionTenureDays, OptedOut: input.Customer.OptedOut}
	if total > 0 {
		profile.SuccessfulPaymentRatio = float64(input.Profile.SuccessfulPayments) / float64(total)
	}
	profile.AverageTransactionValueMinor = int64(number(features["average_transaction_value_minor"]))
	profile.PreferredPaymentMethod, _ = features["preferred_payment_method"].(string)
	profile.PriorCheckoutRecoveries = int(number(features["prior_checkout_recoveries"]))
	recent := make([]RecentAction, 0, len(input.Actions))
	for _, action := range input.Actions {
		if action.Status == "SCHEDULED" {
			continue
		}
		recent = append(recent, RecentAction{Type: action.Type, Status: action.Status, CreatedAt: action.CreatedAt})
		profile.PreviousRecoveryAttempts++
		if isContact(action.Type) {
			profile.ContactCount++
		}
	}
	payment := PaymentState{AlreadyRecovered: input.Case.CurrentState == domain.StateRecovered, AvailableChannels: append([]string(nil), input.Policy.AllowedChannels...), MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID"}
	var failure map[string]any
	_ = json.Unmarshal(input.Case.FailureOrLeakContext, &failure)
	if value, ok := failure["mandate_status"].(string); ok {
		payment.MandateStatus = strings.ToUpper(value)
	}
	if diagnosis.FailureCategory == "PAYMENT_METHOD_INVALID" {
		payment.PaymentMethodStatus = "INVALID"
	}
	merchant := MerchantContext{MerchantType: input.Merchant.Type, RecoveryObjective: input.Policy.Objective,
		AllowedChannels: append([]string(nil), input.Policy.AllowedChannels...), AllowedActions: append([]domain.ActionType(nil), input.Policy.AllowedActions...),
		MaxRetries: input.Policy.MaxRetries, MaxContactsPerDay: input.Policy.MaxContactsPerDay, MaxContactsPerWeek: input.Policy.MaxContactsPerWeek,
		MinimumContactIntervalMinutes: input.Policy.MinContactIntervalMinutes,
		RecoveryWindowHours:           input.Policy.RecoveryWindowHours, QuietHours: input.Policy.QuietHours, MinimumEconomicValueMinor: input.Policy.MinimumEconomicValueMinor,
		HighValueThresholdMinor: input.Policy.HighValueThresholdMinor, LowConfidenceThreshold: input.Policy.LowConfidenceThreshold}
	merchant.MaximumIncentiveMinor = input.Policy.MaximumIncentiveMinor
	if optimization := input.Policy.OptimizationProfile; optimization != nil {
		merchant.OptimizationProfileID = optimization.ID
		merchant.OptimizationProfileVersion = optimization.ConfigurationVersion
		merchant.RecoveryObjective = optimization.Objective
		merchant.RevenueWeightBPS = optimization.RevenueWeightBPS
		merchant.RetentionWeightBPS = optimization.RetentionWeightBPS
		merchant.ContactPenaltyWeightBPS = optimization.ContactPenaltyWeightBPS
		merchant.CostPenaltyWeightBPS = optimization.CostPenaltyWeightBPS
		merchant.FatiguePenaltyWeightBPS = optimization.FatiguePenaltyWeightBPS
		merchant.RiskPenaltyWeightBPS = optimization.RiskPenaltyWeightBPS
		merchant.EscalationPreference = optimization.EscalationPreference
		merchant.MinimumEconomicValueMinor = optimization.MinimumNERVMinor
		if len(optimization.AllowedActions) > 0 {
			merchant.AllowedActions = append([]domain.ActionType(nil), optimization.AllowedActions...)
		}
		if len(optimization.AllowedChannels) > 0 {
			merchant.AllowedChannels = append([]string(nil), optimization.AllowedChannels...)
		}
	}
	var merchantMetadata map[string]any
	_ = json.Unmarshal(input.Merchant.Metadata, &merchantMetadata)
	merchant.Timezone, _ = merchantMetadata["timezone"].(string)
	if merchant.Timezone == "" {
		merchant.Timezone = "UTC"
	}
	result := RecoveryDecisionContext{FeatureVersion: FeatureVersion, Case: input.Case, Diagnosis: diagnosis, CustomerProfile: profile, MerchantContext: merchant,
		RecentActions: recent, ActivePromise: input.ActivePromise, TimingContext: TimingContext{EvaluatedAt: input.Now.UTC(), HoursSinceLeak: input.Now.Sub(input.Case.CreatedAt).Hours(), HoursUntilDeadline: input.Case.RecoveryDeadline.Sub(input.Now).Hours()}, PaymentState: payment}
	encoded, _ := json.Marshal(result)
	if err := RejectHiddenFeatures(encoded); err != nil {
		return RecoveryDecisionContext{}, err
	}
	return result, nil
}

var hiddenFeatureNames = []string{"liquidity_pattern", "retry_responsiveness", "payment_link_responsiveness", "reminder_responsiveness", "contact_sensitivity", "natural_recovery_probability", "churn_intent"}

func RejectHiddenFeatures(encoded []byte) error {
	lower := strings.ToLower(string(encoded))
	for _, name := range hiddenFeatureNames {
		if strings.Contains(lower, `"`+name+`"`) {
			return fmt.Errorf("hidden simulator feature %q cannot enter decision context", name)
		}
	}
	return nil
}
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func number(v any) float64 {
	if n, ok := v.(float64); ok {
		return n
	}
	return 0
}
func isContact(action domain.ActionType) bool {
	switch action {
	case domain.ActionSendReminder, domain.ActionSendPaymentLink, domain.ActionSendCheckoutRecoveryLink, domain.ActionRequestPaymentMethodUpdate, domain.ActionSuggestAlternateMethod, domain.ActionWaitForPromiseToPay, domain.ActionRetention:
		return true
	}
	return false
}

type Repository interface {
	GetCase(context.Context, domain.ID) (domain.RecoveryCase, error)
	LoadContextInputs(context.Context, domain.RecoveryCase) (domain.Customer, domain.CustomerRecoveryProfile, domain.Merchant, domain.MerchantPolicy, []domain.RecoveryAction, *domain.PromiseToPay, error)
}
type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}
func (s *Service) Get(ctx context.Context, caseID domain.ID) (RecoveryDecisionContext, error) {
	c, err := s.repository.GetCase(ctx, caseID)
	if err != nil {
		return RecoveryDecisionContext{}, err
	}
	customer, profile, merchant, policy, actions, promise, err := s.repository.LoadContextInputs(ctx, c)
	if err != nil {
		return RecoveryDecisionContext{}, err
	}
	return Build(BuildInput{Case: c, Customer: customer, Profile: profile, Merchant: merchant, Policy: policy, Actions: actions, ActivePromise: promise, Now: s.now().UTC()})
}
