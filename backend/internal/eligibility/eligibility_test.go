package eligibility

import (
	"context"
	"encoding/json"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

func eligibleContext() recoverycontext.RecoveryDecisionContext {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return recoverycontext.RecoveryDecisionContext{Case: domain.RecoveryCase{LeakType: domain.FailedSubscription, CurrentState: domain.StateActionPending, RecoveryDeadline: now.Add(24 * time.Hour)},
		CustomerProfile: recoverycontext.CustomerProfile{}, TimingContext: recoverycontext.TimingContext{EvaluatedAt: now},
		MerchantContext: recoverycontext.MerchantContext{AllowedActions: []domain.ActionType{domain.ActionRetryNow, domain.ActionRetryLater, domain.ActionSendReminder, domain.ActionSendPaymentLink, domain.ActionRequestPaymentMethodUpdate, domain.ActionSuggestAlternateMethod, domain.ActionWaitForPromiseToPay, domain.ActionRetention}, AllowedChannels: []string{"EMAIL"}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5},
		PaymentState:    recoverycontext.PaymentState{MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID", AvailableChannels: []string{"EMAIL"}}}
}
func exclusionReason(result Result, action domain.ActionType) string {
	for _, excluded := range result.ExcludedActions {
		if excluded.Action == action {
			return excluded.Reason
		}
	}
	return ""
}
func TestEligibilityConditions(t *testing.T) {
	now := eligibleContext().TimingContext.EvaluatedAt
	tests := []struct {
		name   string
		mutate func(*recoverycontext.RecoveryDecisionContext)
		action domain.ActionType
		reason string
	}{
		{"opt_out", func(c *recoverycontext.RecoveryDecisionContext) { c.CustomerProfile.OptedOut = true }, domain.ActionSendReminder, "CUSTOMER_OPT_OUT"},
		{"active_promise", func(c *recoverycontext.RecoveryDecisionContext) {
			c.ActivePromise = &domain.PromiseToPay{Status: "ACTIVE"}
		}, domain.ActionRetryLater, "ACTIVE_PROMISE_TO_PAY"},
		{"quiet_hours", func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.QuietHours = json.RawMessage(`{"start":"11:00","end":"13:00"}`)
		}, domain.ActionSendReminder, "QUIET_HOURS"},
		{"channel", func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.AvailableChannels = []string{"SMS"} }, domain.ActionSendReminder, "CHANNEL_UNAVAILABLE"},
		{"contacts", func(c *recoverycontext.RecoveryDecisionContext) {
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionSendReminder, CreatedAt: now}, {Type: domain.ActionSendPaymentLink, CreatedAt: now}, {Type: domain.ActionRetention, CreatedAt: now}}
		}, domain.ActionSendReminder, "MAX_CONTACTS_REACHED"},
		{"retries", func(c *recoverycontext.RecoveryDecisionContext) {
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionRetryNow}, {Type: domain.ActionRetryLater}, {Type: domain.ActionRetryNow}}
		}, domain.ActionRetryNow, "MAX_RETRIES_REACHED"},
		{"mandate", func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.MandateStatus = "REVOKED" }, domain.ActionRetryLater, "MANDATE_UNAVAILABLE"},
		{"invalid_method", func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.PaymentMethodStatus = "INVALID" }, domain.ActionRetryNow, "PAYMENT_METHOD_INVALID"},
		{"recovered", func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.AlreadyRecovered = true }, domain.ActionRetryNow, "PAYMENT_ALREADY_RECOVERED"},
		{"expired", func(c *recoverycontext.RecoveryDecisionContext) { c.Case.RecoveryDeadline = now.Add(-time.Minute) }, domain.ActionSendPaymentLink, "RECOVERY_WINDOW_EXPIRED"},
		{"cooldown", func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.MinimumContactIntervalMinutes = 60
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionSendReminder, CreatedAt: now.Add(-10 * time.Minute)}}
		}, domain.ActionSendReminder, "ACTION_COOLDOWN"},
		{"retry_cooldown", func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.MinimumContactIntervalMinutes = 60
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionRetryLater, CreatedAt: now.Add(-10 * time.Minute)}}
		}, domain.ActionRetryLater, "RETRY_COOLDOWN"},
		{"vertical", func(c *recoverycontext.RecoveryDecisionContext) {}, domain.ActionSendCheckoutRecoveryLink, "LEAK_TYPE_INCOMPATIBLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := eligibleContext()
			test.mutate(&c)
			result := Evaluate(c)
			if got := exclusionReason(result, test.action); got != test.reason {
				t.Fatalf("got %q want %q: %+v", got, test.reason, result)
			}
		})
	}
}

type spyPredictor struct{ received []domain.ActionType }

func (s *spyPredictor) Predict(_ context.Context, _ recoverycontext.RecoveryDecisionContext, actions []domain.ActionType) error {
	s.received = actions
	return nil
}
func TestExcludedActionsNeverCrossPredictionBoundary(t *testing.T) {
	c := eligibleContext()
	c.CustomerProfile.OptedOut = true
	c.PaymentState.PaymentMethodStatus = "INVALID"
	spy := &spyPredictor{}
	if err := FilterThenPredict(context.Background(), c, spy); err != nil {
		t.Fatal(err)
	}
	for _, action := range spy.received {
		if contactActions[action] || retryActions[action] {
			t.Fatalf("ineligible action %s reached predictor", action)
		}
	}
}
