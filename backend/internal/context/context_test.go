package recoverycontext

import (
	"encoding/json"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

func baseInput() BuildInput {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return BuildInput{Now: now, Case: domain.RecoveryCase{ID: "case", LeakType: domain.FailedSubscription, AmountAtRiskMinor: 10000,
		CurrentState: domain.StateActionPending, CreatedAt: now.Add(-time.Hour), RecoveryDeadline: now.Add(48 * time.Hour),
		FailureOrLeakContext: json.RawMessage(`{"failure_category":"INSUFFICIENT_FUNDS"}`)},
		Customer: domain.Customer{ID: "customer"}, Merchant: domain.Merchant{ID: "merchant", Type: "SAAS"},
		Policy: domain.MerchantPolicy{Objective: "MAXIMIZE_NET_RECOVERY", MaxRetries: 3, MaxContactsPerDay: 2, MaxContactsPerWeek: 5,
			RecoveryWindowHours: 168, AllowedActions: []domain.ActionType{domain.ActionWait, domain.ActionRetryLater}, AllowedChannels: []string{"EMAIL"}, QuietHours: json.RawMessage(`{"start":"21:00","end":"08:00"}`)},
		Profile: domain.CustomerRecoveryProfile{SuccessfulPayments: 20, FailedPayments: 2, SubscriptionTenureDays: 720, PromiseReliability: .8, FatigueScore: .1, Features: json.RawMessage(`{"average_transaction_value_minor":10000,"preferred_payment_method":"UPI"}`)}}
}
func TestDiagnosisNormalizationAndBounds(t *testing.T) {
	d := Diagnose(baseInput().Case)
	if d.FailureCategory != "INSUFFICIENT_FUNDS" || d.Recoverability != "TEMPORARY" || d.Confidence < 0 || d.Confidence > 1 {
		t.Fatalf("%+v", d)
	}
}
func TestSameFailureDifferentHistoryProducesDifferentContext(t *testing.T) {
	a := baseInput()
	b := baseInput()
	b.Profile.SuccessfulPayments = 1
	b.Profile.FailedPayments = 12
	b.Profile.FatigueScore = .9
	b.Profile.PromiseReliability = .2
	b.Actions = []domain.RecoveryAction{{Type: domain.ActionSendReminder}, {Type: domain.ActionRetryLater}, {Type: domain.ActionSendPaymentLink}}
	ca, err := Build(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := Build(b)
	if err != nil {
		t.Fatal(err)
	}
	if ca.Diagnosis.FailureCategory != cb.Diagnosis.FailureCategory || ca.Diagnosis.Recoverability != cb.Diagnosis.Recoverability || ca.Diagnosis.Confidence != cb.Diagnosis.Confidence {
		t.Fatal("diagnosis should be identical")
	}
	if ca.CustomerProfile.SuccessfulPaymentRatio == cb.CustomerProfile.SuccessfulPaymentRatio || ca.CustomerProfile.RecoveryFatigue == cb.CustomerProfile.RecoveryFatigue || cb.CustomerProfile.PreviousRecoveryAttempts != 3 {
		t.Fatalf("contexts not materially different: %+v %+v", ca.CustomerProfile, cb.CustomerProfile)
	}
}
func TestHiddenFeaturesRejected(t *testing.T) {
	input := baseInput()
	input.Profile.Features = json.RawMessage(`{"liquidity_pattern":"STABLE"}`)
	if _, err := Build(input); err == nil {
		t.Fatal("hidden feature leaked")
	}
}
func TestContextIsDeterministic(t *testing.T) {
	input := baseInput()
	a, _ := Build(input)
	b, _ := Build(input)
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	if string(left) != string(right) {
		t.Fatal("context not deterministic")
	}
}
func TestCurrentScheduledActionIsNotCountedAsPastAttempt(t *testing.T) {
	input := baseInput()
	input.Actions = []domain.RecoveryAction{{Type: domain.ActionRetryLater, Status: "SCHEDULED"}, {Type: domain.ActionRetryNow, Status: "FAILED"}}
	result, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RecentActions) != 1 || result.RecentActions[0].Type != domain.ActionRetryNow || result.CustomerProfile.PreviousRecoveryAttempts != 1 {
		t.Fatalf("unexpected action history: %+v", result)
	}
}
