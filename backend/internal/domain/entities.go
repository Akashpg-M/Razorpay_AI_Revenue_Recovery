package domain

import (
	"encoding/json"
	"time"
)

type ID string

type Money struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type Merchant struct {
	ID        ID              `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type MerchantPolicy struct {
	ID                             ID                           `json:"id"`
	MerchantID                     ID                           `json:"merchant_id"`
	Objective                      string                       `json:"objective"`
	MaxRetries                     int                          `json:"max_retries"`
	MaxContactsPerDay              int                          `json:"max_contacts_per_day"`
	MaxContactsPerWeek             int                          `json:"max_contacts_per_week"`
	MinContactIntervalMinutes      int                          `json:"min_contact_interval_minutes"`
	RecoveryWindowHours            int                          `json:"recovery_window_hours"`
	QuietHours                     json.RawMessage              `json:"quiet_hours"`
	AllowedActions                 []ActionType                 `json:"allowed_actions"`
	AllowedChannels                []string                     `json:"allowed_channels"`
	HighValueThresholdMinor        int64                        `json:"high_value_threshold_minor"`
	LowConfidenceThreshold         float64                      `json:"low_confidence_threshold"`
	MinimumEconomicValueMinor      int64                        `json:"minimum_economic_value_minor"`
	MaximumIncentiveMinor          int64                        `json:"maximum_incentive_minor"`
	RequiresHighValueHumanApproval bool                         `json:"requires_high_value_human_approval"`
	Version                        int                          `json:"version"`
	CreatedAt                      time.Time                    `json:"created_at"`
	UpdatedAt                      time.Time                    `json:"updated_at"`
	OptimizationProfile            *MerchantOptimizationProfile `json:"optimization_profile,omitempty"`
}

type Customer struct {
	ID         ID              `json:"id"`
	MerchantID ID              `json:"merchant_id"`
	ExternalID string          `json:"external_id"`
	Contact    json.RawMessage `json:"contact"`
	OptedOut   bool            `json:"opted_out"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type CustomerRecoveryProfile struct {
	ID                     ID              `json:"id"`
	CustomerID             ID              `json:"customer_id"`
	SuccessfulPayments     int             `json:"successful_payments"`
	FailedPayments         int             `json:"failed_payments"`
	SubscriptionTenureDays int             `json:"subscription_tenure_days"`
	PromiseReliability     float64         `json:"promise_reliability"`
	FatigueScore           float64         `json:"fatigue_score"`
	Features               json.RawMessage `json:"features"`
	Version                int             `json:"version"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type LeakType string

const (
	FailedSubscription  LeakType = "FAILED_SUBSCRIPTION"
	CheckoutAbandonment LeakType = "CHECKOUT_ABANDONMENT"
)

type RecoveryCase struct {
	ID                      ID              `json:"case_id"`
	LeakType                LeakType        `json:"leak_type"`
	MerchantID              ID              `json:"merchant_id"`
	CustomerID              ID              `json:"customer_id"`
	AmountAtRiskMinor       int64           `json:"amount_at_risk_minor"`
	Currency                string          `json:"currency"`
	SourceReference         string          `json:"source_reference"`
	SourceStatus            string          `json:"source_status"`
	FailureOrLeakContext    json.RawMessage `json:"failure_or_leak_context"`
	CustomerContextSnapshot json.RawMessage `json:"customer_context_snapshot"`
	MerchantPolicySnapshot  json.RawMessage `json:"merchant_policy_snapshot"`
	CurrentState            CaseState       `json:"current_state"`
	RecoveryDeadline        time.Time       `json:"recovery_deadline"`
	RecoveredAmountMinor    int64           `json:"recovered_amount_minor"`
	AttributionStatus       string          `json:"attribution_status"`
	Version                 int64           `json:"version"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type ActionType string

const (
	ActionWait                       ActionType = "WAIT"
	ActionRetryNow                   ActionType = "RETRY_NOW"
	ActionRetryLater                 ActionType = "RETRY_LATER"
	ActionSendReminder               ActionType = "SEND_REMINDER"
	ActionSendPaymentLink            ActionType = "SEND_PAYMENT_LINK"
	ActionSendCheckoutRecoveryLink   ActionType = "SEND_CHECKOUT_RECOVERY_LINK"
	ActionRequestPaymentMethodUpdate ActionType = "REQUEST_PAYMENT_METHOD_UPDATE"
	ActionSuggestAlternateMethod     ActionType = "SUGGEST_ALTERNATE_METHOD"
	ActionWaitForPromiseToPay        ActionType = "WAIT_FOR_PROMISE_TO_PAY"
	ActionRetention                  ActionType = "RETENTION_ACTION"
	ActionEscalateToHuman            ActionType = "ESCALATE_TO_HUMAN"
	ActionStop                       ActionType = "STOP"
)

type RecoveryAction struct {
	ID               ID              `json:"action_id"`
	CaseID           ID              `json:"case_id"`
	Type             ActionType      `json:"action_type"`
	Status           string          `json:"status"`
	Parameters       json.RawMessage `json:"parameters"`
	IdempotencyKey   string          `json:"idempotency_key"`
	PolicyDecisionID *ID             `json:"policy_decision_id,omitempty"`
	ScheduledAt      *time.Time      `json:"scheduled_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type ActionPrediction struct {
	ID                         ID              `json:"prediction_id"`
	CaseID                     ID              `json:"case_id"`
	ActionID                   *ID             `json:"action_id,omitempty"`
	ActionType                 ActionType      `json:"action_type"`
	RecoveryProbability        float64         `json:"recovery_probability"`
	NaturalRecoveryProbability float64         `json:"natural_recovery_probability"`
	IncrementalUplift          float64         `json:"incremental_uplift"`
	ExpectedNetValueMinor      int64           `json:"expected_net_value_minor"`
	ModelVersionID             *ID             `json:"model_version_id,omitempty"`
	ModelVersion               string          `json:"model_version"`
	FeatureVersion             string          `json:"feature_version"`
	Explanation                json.RawMessage `json:"explanation"`
	CreatedAt                  time.Time       `json:"created_at"`
}

type PolicyDecision struct {
	ID            ID              `json:"policy_decision_id"`
	CaseID        ID              `json:"case_id"`
	ActionID      *ID             `json:"action_id,omitempty"`
	Decision      string          `json:"decision"`
	ReasonCodes   []string        `json:"reason_codes"`
	PolicyVersion int             `json:"policy_version"`
	Snapshot      json.RawMessage `json:"snapshot"`
	CreatedAt     time.Time       `json:"created_at"`
}

type Execution struct {
	ID                ID              `json:"execution_id"`
	CaseID            ID              `json:"case_id"`
	ActionID          ID              `json:"action_id"`
	Attempt           int             `json:"attempt"`
	Status            string          `json:"status"`
	ProviderReference string          `json:"provider_reference"`
	Request           json.RawMessage `json:"request"`
	Response          json.RawMessage `json:"response"`
	StartedAt         time.Time       `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

type PromiseToPay struct {
	ID                    ID              `json:"promise_id"`
	CaseID                ID              `json:"case_id"`
	CustomerID            ID              `json:"customer_id"`
	Status                string          `json:"status"`
	DueAt                 time.Time       `json:"due_at"`
	Confidence            float64         `json:"confidence"`
	Source                json.RawMessage `json:"source"`
	CreatedAt             time.Time       `json:"created_at"`
	ResolvedAt            *time.Time      `json:"resolved_at,omitempty"`
	PromisedAmountMinor   *int64          `json:"promised_amount_minor,omitempty"`
	ExtractorVersion      string          `json:"extractor_version"`
	ExtractionTimestamp   *time.Time      `json:"extraction_timestamp,omitempty"`
	SourceResponseID      *ID             `json:"source_response_id,omitempty"`
	FulfilledAt           *time.Time      `json:"fulfilled_at,omitempty"`
	BrokenAt              *time.Time      `json:"broken_at,omitempty"`
	ExpiredAt             *time.Time      `json:"expired_at,omitempty"`
	CancelledAt           *time.Time      `json:"cancelled_at,omitempty"`
	VerificationReference string          `json:"verification_reference,omitempty"`
}

type MerchantOptimizationProfile struct {
	ID                      ID           `json:"profile_id"`
	MerchantID              ID           `json:"merchant_id"`
	Objective               string       `json:"objective"`
	RevenueWeightBPS        int64        `json:"revenue_weight_bps"`
	RetentionWeightBPS      int64        `json:"retention_weight_bps"`
	ContactPenaltyWeightBPS int64        `json:"contact_penalty_weight_bps"`
	CostPenaltyWeightBPS    int64        `json:"cost_penalty_weight_bps"`
	FatiguePenaltyWeightBPS int64        `json:"fatigue_penalty_weight_bps"`
	RiskPenaltyWeightBPS    int64        `json:"risk_penalty_weight_bps"`
	EscalationPreference    string       `json:"escalation_preference"`
	AllowedActions          []ActionType `json:"allowed_actions"`
	AllowedChannels         []string     `json:"allowed_channels"`
	MinimumNERVMinor        int64        `json:"minimum_nerv_minor"`
	DiscountBudgetMinor     int64        `json:"discount_budget_minor"`
	HumanReviewBudgetMinor  int64        `json:"human_review_budget_minor"`
	ConfigurationVersion    int          `json:"configuration_version"`
	CreatedAt               time.Time    `json:"created_at"`
}

type WebhookEvent struct {
	ID              ID              `json:"webhook_event_id"`
	Provider        string          `json:"provider"`
	ProviderEventID string          `json:"provider_event_id"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	Status          string          `json:"status"`
	ReceivedAt      time.Time       `json:"received_at"`
	ProcessedAt     *time.Time      `json:"processed_at,omitempty"`
}

type ModelVersion struct {
	ID             ID              `json:"model_version_id"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	FeatureVersion string          `json:"feature_version"`
	Metrics        json.RawMessage `json:"metrics"`
	ArtifactURI    string          `json:"artifact_uri"`
	CreatedAt      time.Time       `json:"created_at"`
}

type EvaluationRun struct {
	ID                ID              `json:"evaluation_run_id"`
	SimulationVersion string          `json:"simulation_version"`
	Seed              int64           `json:"seed"`
	DatasetSize       int             `json:"dataset_size"`
	SplitIdentifiers  json.RawMessage `json:"split_identifiers"`
	ModelVersionID    *ID             `json:"model_version_id,omitempty"`
	FeatureVersion    string          `json:"feature_version"`
	StrategyVersion   string          `json:"strategy_version"`
	PolicyVersion     string          `json:"policy_version"`
	Metrics           json.RawMessage `json:"metrics"`
	CreatedAt         time.Time       `json:"created_at"`
}
