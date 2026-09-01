package eligibility

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/domain"
)

type Exclusion struct {
	Action      domain.ActionType `json:"action"`
	Reason      string            `json:"reason"`
	Explanation string            `json:"explanation,omitempty"`
}
type Result struct {
	EligibleActions []domain.ActionType `json:"eligible_actions"`
	ExcludedActions []Exclusion         `json:"excluded_actions"`
	EvaluatedAt     time.Time           `json:"evaluated_at"`
}

var allActions = []domain.ActionType{domain.ActionWait, domain.ActionRetryNow, domain.ActionRetryLater, domain.ActionSendReminder, domain.ActionSendPaymentLink,
	domain.ActionSendCheckoutRecoveryLink, domain.ActionRequestPaymentMethodUpdate, domain.ActionSuggestAlternateMethod, domain.ActionWaitForPromiseToPay,
	domain.ActionRetention, domain.ActionEscalateToHuman, domain.ActionStop}
var contactActions = map[domain.ActionType]bool{domain.ActionSendReminder: true, domain.ActionSendPaymentLink: true, domain.ActionSendCheckoutRecoveryLink: true,
	domain.ActionRequestPaymentMethodUpdate: true, domain.ActionSuggestAlternateMethod: true, domain.ActionWaitForPromiseToPay: true, domain.ActionRetention: true}
var retryActions = map[domain.ActionType]bool{domain.ActionRetryNow: true, domain.ActionRetryLater: true}

func Evaluate(ctx recoverycontext.RecoveryDecisionContext) Result {
	now := ctx.TimingContext.EvaluatedAt
	result := Result{EvaluatedAt: now, EligibleActions: []domain.ActionType{}, ExcludedActions: []Exclusion{}}
	allowed := set(ctx.MerchantContext.AllowedActions)
	channels := intersection(ctx.MerchantContext.AllowedChannels, ctx.PaymentState.AvailableChannels)
	actionCount := counts(ctx.RecentActions, now)
	retries := actionCount.retries
	contacts := actionCount
	inQuiet := quietHours(ctx.MerchantContext.QuietHours, now)
	for _, action := range allActions {
		reason, explanation := "", ""
		switch {
		case ctx.PaymentState.AlreadyRecovered || ctx.Case.CurrentState == domain.StateRecovered:
			if action != domain.ActionStop {
				reason = "PAYMENT_ALREADY_RECOVERED"
			}
		case !ctx.Case.RecoveryDeadline.After(now):
			if action != domain.ActionStop {
				reason = "RECOVERY_WINDOW_EXPIRED"
			}
		case !compatible(ctx.Case.LeakType, action):
			reason = "LEAK_TYPE_INCOMPATIBLE"
		case action != domain.ActionStop && action != domain.ActionEscalateToHuman && action != domain.ActionWait && !allowed[action]:
			reason = "MERCHANT_ACTION_NOT_ALLOWED"
		case ctx.ActivePromise != nil && ctx.ActivePromise.Status == "ACTIVE" && (retryActions[action] || contactActions[action]) && action != domain.ActionWaitForPromiseToPay:
			reason = "ACTIVE_PROMISE_TO_PAY"
		case ctx.CustomerProfile.OptedOut && contactActions[action]:
			reason = "CUSTOMER_OPT_OUT"
		case contactActions[action] && len(channels) == 0:
			reason = "CHANNEL_UNAVAILABLE"
		case contactActions[action] && inQuiet:
			reason = "QUIET_HOURS"
		case contactActions[action] && (contacts.day >= ctx.MerchantContext.MaxContactsPerDay || contacts.week >= ctx.MerchantContext.MaxContactsPerWeek):
			reason = "MAX_CONTACTS_REACHED"
		case retryActions[action] && retries >= ctx.MerchantContext.MaxRetries:
			reason = "MAX_RETRIES_REACHED"
		case retryActions[action] && (ctx.PaymentState.MandateStatus == "REVOKED" || ctx.PaymentState.MandateStatus == "FAILED"):
			reason = "MANDATE_UNAVAILABLE"
		case retryActions[action] && ctx.PaymentState.PaymentMethodStatus == "INVALID":
			reason = "PAYMENT_METHOD_INVALID"
		case retryActions[action] && inCooldown(ctx.RecentActions, action, now, ctx.MerchantContext.MinimumContactIntervalMinutes):
			reason = "RETRY_COOLDOWN"
		case contactActions[action] && inCooldown(ctx.RecentActions, action, now, ctx.MerchantContext.MinimumContactIntervalMinutes):
			reason = "ACTION_COOLDOWN"
		}
		if reason != "" {
			result.ExcludedActions = append(result.ExcludedActions, Exclusion{Action: action, Reason: reason, Explanation: explanation})
		} else {
			result.EligibleActions = append(result.EligibleActions, action)
		}
	}
	return result
}

func compatible(leak domain.LeakType, action domain.ActionType) bool {
	if action == domain.ActionRetryNow || action == domain.ActionRetryLater || action == domain.ActionRequestPaymentMethodUpdate || action == domain.ActionWaitForPromiseToPay {
		return leak == domain.FailedSubscription
	}
	if action == domain.ActionSendCheckoutRecoveryLink {
		return leak == domain.CheckoutAbandonment
	}
	return true
}
func set(values []domain.ActionType) map[domain.ActionType]bool {
	r := map[domain.ActionType]bool{}
	for _, v := range values {
		r[v] = true
	}
	return r
}
func intersection(a, b []string) []string {
	right := map[string]bool{}
	for _, v := range b {
		right[v] = true
	}
	var result []string
	for _, v := range a {
		if right[v] {
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

type actionCounts struct{ day, week, retries int }

func counts(actions []recoverycontext.RecentAction, now time.Time) actionCounts {
	var c actionCounts
	for _, a := range actions {
		if retryActions[a.Type] {
			c.retries++
		}
		if contactActions[a.Type] {
			age := now.Sub(a.CreatedAt)
			if age <= 24*time.Hour {
				c.day++
			}
			if age <= 7*24*time.Hour {
				c.week++
			}
		}
	}
	return c
}
func inCooldown(actions []recoverycontext.RecentAction, action domain.ActionType, now time.Time, minutes int) bool {
	if minutes <= 0 {
		return false
	}
	for _, a := range actions {
		if a.Type == action && now.Sub(a.CreatedAt) < time.Duration(minutes)*time.Minute {
			return true
		}
	}
	return false
}
func quietHours(raw json.RawMessage, now time.Time) bool {
	var q struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if json.Unmarshal(raw, &q) != nil || q.Start == "" || q.End == "" {
		return false
	}
	start, err1 := time.Parse("15:04", q.Start)
	end, err2 := time.Parse("15:04", q.End)
	if err1 != nil || err2 != nil {
		return false
	}
	minutes := now.Hour()*60 + now.Minute()
	s := start.Hour()*60 + start.Minute()
	e := end.Hour()*60 + end.Minute()
	if s <= e {
		return minutes >= s && minutes < e
	}
	return minutes >= s || minutes < e
}

type ContextProvider interface {
	Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error)
}
type Service struct{ contexts ContextProvider }

func NewService(contexts ContextProvider) *Service { return &Service{contexts: contexts} }
func (s *Service) Get(ctx context.Context, caseID domain.ID) (Result, error) {
	decisionContext, err := s.contexts.Get(ctx, caseID)
	if err != nil {
		return Result{}, err
	}
	return Evaluate(decisionContext), nil
}

type Predictor interface {
	Predict(context.Context, recoverycontext.RecoveryDecisionContext, []domain.ActionType) error
}

func FilterThenPredict(ctx context.Context, decisionContext recoverycontext.RecoveryDecisionContext, predictor Predictor) error {
	result := Evaluate(decisionContext)
	if len(result.EligibleActions) == 0 {
		return fmt.Errorf("no eligible actions")
	}
	return predictor.Predict(ctx, decisionContext, result.EligibleActions)
}
