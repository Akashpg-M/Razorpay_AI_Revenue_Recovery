package optimizer

import (
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

func optContext() recoverycontext.RecoveryDecisionContext {
	return recoverycontext.RecoveryDecisionContext{Case: domain.RecoveryCase{AmountAtRiskMinor: 800000}, Diagnosis: recoverycontext.Diagnosis{Confidence: .9}, CustomerProfile: recoverycontext.CustomerProfile{RecoveryFatigue: .1}, MerchantContext: recoverycontext.MerchantContext{RecoveryObjective: "MAXIMIZE_NET_RECOVERY", MaximumIncentiveMinor: 40000}}
}
func TestIncrementalArithmeticAndNegativeUpliftPreserved(t *testing.T) {
	r := Rank(optContext(), []decisionclient.OutcomePrediction{{Action: "RETRY_LATER", RecoveryProbability: .64}, {Action: "SEND_REMINDER", RecoveryProbability: .30}}, .35, DefaultCostModel(), time.Now())
	var retry, reminder Candidate
	for _, c := range r.Candidates {
		if c.Action == domain.ActionRetryLater {
			retry = c
		}
		if c.Action == domain.ActionSendReminder {
			reminder = c
		}
	}
	if retry.IncrementalUplift < .289999 || retry.GrossIncrementalValueMinor != 232000 {
		t.Fatalf("%+v", retry)
	}
	if reminder.IncrementalUplift >= 0 {
		t.Fatal("negative uplift was clamped")
	}
}
func TestWaitWinsWhenInterventionsAddNoValue(t *testing.T) {
	r := Rank(optContext(), []decisionclient.OutcomePrediction{{Action: "RETRY_LATER", RecoveryProbability: .6}, {Action: "SEND_REMINDER", RecoveryProbability: .58}}, .65, DefaultCostModel(), time.Now())
	if r.Selected.Action != domain.ActionWait {
		t.Fatalf("selected %s", r.Selected.Action)
	}
}
func TestCheaperActionAndObjectiveRecorded(t *testing.T) {
	ctx := optContext()
	pred := []decisionclient.OutcomePrediction{{Action: "SEND_REMINDER", RecoveryProbability: .55}, {Action: "RETENTION_ACTION", RecoveryProbability: .55}}
	r := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	if r.Selected.Action != domain.ActionSendReminder {
		t.Fatalf("selected %s", r.Selected.Action)
	}
	ctx.MerchantContext.RecoveryObjective = "MINIMIZE_CONTACT"
	contact := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	if contact.MerchantObjective != "MINIMIZE_CONTACT" {
		t.Fatal("objective not recorded")
	}
}
func TestMonetaryPrecision(t *testing.T) {
	if got := probabilityValue(101, .333333); got != 33 {
		t.Fatalf("got %d", got)
	}
}
func TestMerchantObjectiveCanChangeWinner(t *testing.T) {
	ctx := optContext()
	ctx.Case.AmountAtRiskMinor = 100000
	ctx.CustomerProfile.RecoveryFatigue = 1
	pred := []decisionclient.OutcomePrediction{{Action: "SEND_REMINDER", RecoveryProbability: .51}, {Action: "RETRY_LATER", RecoveryProbability: .499}}
	net := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	if net.Selected.Action != domain.ActionSendReminder {
		t.Fatalf("net objective selected %s", net.Selected.Action)
	}
	ctx.MerchantContext.RecoveryObjective = "MINIMIZE_CONTACT"
	contact := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	if contact.Selected.Action != domain.ActionRetryLater {
		t.Fatalf("contact objective selected %s", contact.Selected.Action)
	}
}

func TestVersionedMerchantProfileWeightsChangeRanking(t *testing.T) {
	ctx := optContext()
	ctx.Case.AmountAtRiskMinor = 100000
	ctx.CustomerProfile.RecoveryFatigue = 1
	ctx.MerchantContext.OptimizationProfileVersion = 3
	ctx.MerchantContext.RevenueWeightBPS = 10000
	pred := []decisionclient.OutcomePrediction{{Action: "SEND_REMINDER", RecoveryProbability: .51}, {Action: "RETRY_LATER", RecoveryProbability: .499}}
	revenue := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	ctx.MerchantContext.RevenueWeightBPS = 1000
	ctx.MerchantContext.FatiguePenaltyWeightBPS = 20000
	contactSensitive := Rank(ctx, pred, .3, DefaultCostModel(), time.Now())
	if revenue.Selected.Action != domain.ActionSendReminder || contactSensitive.Selected.Action != domain.ActionRetryLater {
		t.Fatalf("profile weights did not alter ranking: revenue=%s contact=%s", revenue.Selected.Action, contactSensitive.Selected.Action)
	}
}
