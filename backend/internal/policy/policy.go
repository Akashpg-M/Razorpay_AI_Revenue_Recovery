package policy

import (
	"encoding/json"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/optimizer"
	"time"
)

const Version = "policy-v1"

type Result struct {
	ID             domain.ID         `json:"policy_evaluation_id"`
	DecisionID     domain.ID         `json:"decision_id"`
	EconomicGateID domain.ID         `json:"economic_gate_id"`
	CaseID         domain.ID         `json:"case_id"`
	CaseVersion    int64             `json:"case_version"`
	SelectedAction domain.ActionType `json:"selected_action"`
	PolicyVersion  string            `json:"policy_version"`
	Decision       string            `json:"decision"`
	ReasonCodes    []string          `json:"reason_codes"`
	Checks         map[string]bool   `json:"checks"`
	CreatedAt      time.Time         `json:"created_at"`
}

var contacts = map[domain.ActionType]bool{domain.ActionSendReminder: true, domain.ActionSendPaymentLink: true, domain.ActionSendCheckoutRecoveryLink: true, domain.ActionRequestPaymentMethodUpdate: true, domain.ActionSuggestAlternateMethod: true, domain.ActionRetention: true}
var retries = map[domain.ActionType]bool{domain.ActionRetryNow: true, domain.ActionRetryLater: true}

func Evaluate(ctx recoverycontext.RecoveryDecisionContext, decisionID domain.ID, decisionCaseVersion int64, candidate optimizer.Candidate, gate economicgate.Result, now time.Time) Result {
	stop, deny, escalate := []string{}, []string{}, []string{}
	checks := map[string]bool{}
	add := func(target *[]string, reason string, condition bool) {
		checks[reason] = !condition
		if condition {
			*target = append(*target, reason)
		}
	}
	add(&stop, "PAYMENT_ALREADY_RECOVERED", ctx.PaymentState.AlreadyRecovered || ctx.Case.CurrentState == domain.StateRecovered)
	add(&stop, "TERMINAL_CASE_STATE", ctx.Case.CurrentState.IsTerminal())
	add(&stop, "SUBSCRIPTION_CANCELLED", ctx.Case.SourceStatus == "CANCELLED")
	add(&stop, "RECOVERY_WINDOW_EXPIRED", !ctx.Case.RecoveryDeadline.After(now))
	add(&deny, "STALE_DECISION", ctx.Case.Version != decisionCaseVersion)
	add(&deny, "ECONOMIC_GATE_BLOCKED", gate.Decision != "ALLOW")
	add(&deny, "CUSTOMER_OPT_OUT", ctx.CustomerProfile.OptedOut && contacts[candidate.Action])
	retryCount, dayContacts, weekContacts, lastContact, lastRetry := history(ctx.RecentActions, now)
	add(&deny, "MAX_RETRIES", retries[candidate.Action] && retryCount >= ctx.MerchantContext.MaxRetries)
	add(&deny, "KNOWN_PERMANENT_FAILURE", retries[candidate.Action] && ctx.Diagnosis.Recoverability == "NON_RECOVERABLE")
	add(&deny, "RETRY_COOLDOWN", retries[candidate.Action] && !lastRetry.IsZero() && now.Sub(lastRetry) < time.Duration(ctx.MerchantContext.MinimumContactIntervalMinutes)*time.Minute)
	add(&deny, "MAX_CONTACTS_PER_DAY", contacts[candidate.Action] && dayContacts >= ctx.MerchantContext.MaxContactsPerDay)
	add(&deny, "MAX_CONTACTS_PER_WEEK", contacts[candidate.Action] && weekContacts >= ctx.MerchantContext.MaxContactsPerWeek)
	add(&deny, "MIN_CONTACT_INTERVAL", contacts[candidate.Action] && !lastContact.IsZero() && now.Sub(lastContact) < time.Duration(ctx.MerchantContext.MinimumContactIntervalMinutes)*time.Minute)
	add(&deny, "ACTIVE_PROMISE_TO_PAY", ctx.ActivePromise != nil && ctx.ActivePromise.Status == "ACTIVE" && (contacts[candidate.Action] || retries[candidate.Action]))
	add(&deny, "MANDATE_INVALID", retries[candidate.Action] && (ctx.PaymentState.MandateStatus == "REVOKED" || ctx.PaymentState.MandateStatus == "FAILED"))
	add(&deny, "PAYMENT_METHOD_INVALID", retries[candidate.Action] && ctx.PaymentState.PaymentMethodStatus == "INVALID")
	add(&deny, "MAX_DISCOUNT", candidate.IncentiveCostMinor > ctx.MerchantContext.MaximumIncentiveMinor && ctx.MerchantContext.MaximumIncentiveMinor >= 0)
	add(&deny, "QUIET_HOURS", contacts[candidate.Action] && inQuietHours(ctx.MerchantContext.QuietHours, ctx.MerchantContext.Timezone, now))
	add(&deny, "CHANNEL_UNAVAILABLE", contacts[candidate.Action] && len(ctx.PaymentState.AvailableChannels) == 0)
	add(&escalate, "HIGH_VALUE_APPROVAL", ctx.MerchantContext.HighValueThresholdMinor > 0 && ctx.Case.AmountAtRiskMinor >= ctx.MerchantContext.HighValueThresholdMinor)
	add(&escalate, "LOW_CONFIDENCE_ESCALATION", ctx.Diagnosis.Confidence < ctx.MerchantContext.LowConfidenceThreshold)
	decision := "APPROVE"
	reasons := []string{"ALL_POLICY_CHECKS_PASSED"}
	if len(stop) > 0 {
		decision = "STOP"
		reasons = stop
	} else if len(deny) > 0 {
		decision = "DENY"
		reasons = deny
	} else if len(escalate) > 0 {
		decision = "ESCALATE"
		reasons = escalate
	}
	return Result{DecisionID: decisionID, CaseID: ctx.Case.ID, CaseVersion: ctx.Case.Version, SelectedAction: candidate.Action, PolicyVersion: Version, Decision: decision, ReasonCodes: reasons, Checks: checks, CreatedAt: now.UTC()}
}
func history(actions []recoverycontext.RecentAction, now time.Time) (retry, day, week int, lastContact, lastRetry time.Time) {
	for _, action := range actions {
		if retries[action.Type] {
			retry++
			if action.CreatedAt.After(lastRetry) {
				lastRetry = action.CreatedAt
			}
		}
		if contacts[action.Type] {
			age := now.Sub(action.CreatedAt)
			if age <= 24*time.Hour {
				day++
			}
			if age <= 7*24*time.Hour {
				week++
			}
			if action.CreatedAt.After(lastContact) {
				lastContact = action.CreatedAt
			}
		}
	}
	return
}
func inQuietHours(raw json.RawMessage, timezone string, now time.Time) bool {
	var q struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if json.Unmarshal(raw, &q) != nil || q.Start == "" || q.End == "" {
		return false
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return true
	}
	local := now.In(location)
	start, e1 := time.Parse("15:04", q.Start)
	end, e2 := time.Parse("15:04", q.End)
	if e1 != nil || e2 != nil {
		return true
	}
	minute := local.Hour()*60 + local.Minute()
	s := start.Hour()*60 + start.Minute()
	e := end.Hour()*60 + end.Minute()
	if s <= e {
		return minute >= s && minute < e
	}
	return minute >= s || minute < e
}
