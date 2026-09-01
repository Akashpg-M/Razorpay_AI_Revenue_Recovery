package policy

import (
	"encoding/json"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/optimizer"
	"testing"
	"time"
)

func policyContext() recoverycontext.RecoveryDecisionContext {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return recoverycontext.RecoveryDecisionContext{Case: domain.RecoveryCase{ID: "c", Version: 3, AmountAtRiskMinor: 10000, CurrentState: domain.StateActionPending, RecoveryDeadline: now.Add(time.Hour)}, Diagnosis: recoverycontext.Diagnosis{Confidence: .9}, MerchantContext: recoverycontext.MerchantContext{MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5, MaximumIncentiveMinor: 1000, HighValueThresholdMinor: 100000, LowConfidenceThreshold: .5, Timezone: "UTC"}, PaymentState: recoverycontext.PaymentState{MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID", AvailableChannels: []string{"EMAIL"}}}
}
func allowedGate() economicgate.Result { return economicgate.Result{Decision: "ALLOW"} }
func TestPolicyPrecedenceAndRules(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name, want string
		mutate     func(*recoverycontext.RecoveryDecisionContext)
		candidate  optimizer.Candidate
		version    int64
		gate       economicgate.Result
	}{{"approve", "APPROVE", func(*recoverycontext.RecoveryDecisionContext) {}, optimizer.Candidate{Action: domain.ActionRetryLater}, 3, allowedGate()}, {"optout", "DENY", func(c *recoverycontext.RecoveryDecisionContext) { c.CustomerProfile.OptedOut = true }, optimizer.Candidate{Action: domain.ActionSendReminder}, 3, allowedGate()}, {"high_value", "ESCALATE", func(c *recoverycontext.RecoveryDecisionContext) { c.Case.AmountAtRiskMinor = 100000 }, optimizer.Candidate{Action: domain.ActionRetryLater}, 3, allowedGate()}, {"low_confidence", "ESCALATE", func(c *recoverycontext.RecoveryDecisionContext) { c.Diagnosis.Confidence = .2 }, optimizer.Candidate{Action: domain.ActionRetryLater}, 3, allowedGate()}, {"stale", "DENY", func(*recoverycontext.RecoveryDecisionContext) {}, optimizer.Candidate{Action: domain.ActionRetryLater}, 2, allowedGate()}, {"cancelled", "STOP", func(c *recoverycontext.RecoveryDecisionContext) {
		c.Case.SourceStatus = "CANCELLED"
		c.CustomerProfile.OptedOut = true
	}, optimizer.Candidate{Action: domain.ActionSendReminder}, 3, allowedGate()}, {"economic", "DENY", func(*recoverycontext.RecoveryDecisionContext) {}, optimizer.Candidate{Action: domain.ActionRetryLater}, 3, economicgate.Result{Decision: "BLOCK"}}, {"quiet_timezone", "DENY", func(c *recoverycontext.RecoveryDecisionContext) {
		c.MerchantContext.QuietHours = json.RawMessage(`{"start":"17:00","end":"19:00"}`)
		c.MerchantContext.Timezone = "Asia/Kolkata"
	}, optimizer.Candidate{Action: domain.ActionSendReminder}, 3, allowedGate()}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := policyContext()
			tc.mutate(&c)
			r := Evaluate(c, "d", tc.version, tc.candidate, tc.gate, now)
			if r.Decision != tc.want {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestEveryMajorDenialRuleReturnsMachineReadableReason(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name, reason string
		action       domain.ActionType
		mutate       func(*recoverycontext.RecoveryDecisionContext)
	}{
		{"max retries", "MAX_RETRIES", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) {
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionRetryNow}, {Type: domain.ActionRetryLater}, {Type: domain.ActionRetryNow}}
		}},
		{"permanent failure", "KNOWN_PERMANENT_FAILURE", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) { c.Diagnosis.Recoverability = "NON_RECOVERABLE" }},
		{"retry cooldown", "RETRY_COOLDOWN", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.MinimumContactIntervalMinutes = 60
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionRetryNow, CreatedAt: now.Add(-time.Minute)}}
		}},
		{"mandate invalid", "MANDATE_INVALID", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.MandateStatus = "REVOKED" }},
		{"method invalid", "PAYMENT_METHOD_INVALID", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.PaymentMethodStatus = "INVALID" }},
		{"daily contact", "MAX_CONTACTS_PER_DAY", domain.ActionSendReminder, func(c *recoverycontext.RecoveryDecisionContext) {
			for i := 0; i < 3; i++ {
				c.RecentActions = append(c.RecentActions, recoverycontext.RecentAction{Type: domain.ActionSendReminder, CreatedAt: now.Add(-time.Hour)})
			}
		}},
		{"weekly contact", "MAX_CONTACTS_PER_WEEK", domain.ActionSendReminder, func(c *recoverycontext.RecoveryDecisionContext) {
			for i := 0; i < 5; i++ {
				c.RecentActions = append(c.RecentActions, recoverycontext.RecentAction{Type: domain.ActionSendReminder, CreatedAt: now.Add(-48 * time.Hour)})
			}
		}},
		{"contact interval", "MIN_CONTACT_INTERVAL", domain.ActionSendReminder, func(c *recoverycontext.RecoveryDecisionContext) {
			c.MerchantContext.MinimumContactIntervalMinutes = 60
			c.RecentActions = []recoverycontext.RecentAction{{Type: domain.ActionSendReminder, CreatedAt: now.Add(-time.Minute)}}
		}},
		{"active promise", "ACTIVE_PROMISE_TO_PAY", domain.ActionRetryLater, func(c *recoverycontext.RecoveryDecisionContext) {
			c.ActivePromise = &domain.PromiseToPay{Status: "ACTIVE"}
		}},
		{"channel unavailable", "CHANNEL_UNAVAILABLE", domain.ActionSendReminder, func(c *recoverycontext.RecoveryDecisionContext) { c.PaymentState.AvailableChannels = nil }},
		{"max incentive", "MAX_DISCOUNT", domain.ActionRetention, func(*recoverycontext.RecoveryDecisionContext) {}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := policyContext()
			tc.mutate(&c)
			candidate := optimizer.Candidate{Action: tc.action}
			if tc.reason == "MAX_DISCOUNT" {
				candidate.IncentiveCostMinor = 1001
			}
			result := Evaluate(c, "d", 3, candidate, allowedGate(), now)
			found := false
			for _, reason := range result.ReasonCodes {
				found = found || reason == tc.reason
			}
			if result.Decision != "DENY" || !found {
				t.Fatalf("expected %s: %+v", tc.reason, result)
			}
		})
	}
}
