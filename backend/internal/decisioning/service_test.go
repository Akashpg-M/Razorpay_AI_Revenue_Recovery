package decisioning

import (
	"context"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

type contextStub struct {
	value recoverycontext.RecoveryDecisionContext
}

func (c contextStub) Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error) {
	return c.value, nil
}

type predictorStub struct{ naturalCalls int }

func (p *predictorStub) PredictNatural(context.Context, any) (decisionclient.NaturalPredictionResponse, error) {
	p.naturalCalls++
	return decisionclient.NaturalPredictionResponse{NaturalRecoveryProbability: .35, ModelVersion: "natural-recovery-v1", FeatureVersion: "natural-features-v1", PredictionTimestamp: time.Now()}, nil
}
func (p *predictorStub) PredictOutcomes(_ context.Context, r decisionclient.PredictionRequest) (decisionclient.PredictionResponse, error) {
	result := []decisionclient.OutcomePrediction{}
	for _, a := range r.EligibleActions {
		prob := .45
		if a == "RETRY_LATER" {
			prob = .64
		}
		result = append(result, decisionclient.OutcomePrediction{Action: a, RecoveryProbability: prob, ModelVersion: "outcome-v1", FeatureVersion: "features-v1", PredictionTimestamp: time.Now()})
	}
	return decisionclient.PredictionResponse{Predictions: result}, nil
}

type storeStub struct{ snapshot Snapshot }

func (s *storeStub) SaveDecision(_ context.Context, v Snapshot) error { s.snapshot = v; return nil }
func TestCompleteDecisionUsesOneNaturalProbabilityAndPersists(t *testing.T) {
	now := time.Now()
	ctx := recoverycontext.RecoveryDecisionContext{FeatureVersion: "recovery-context-v1", Case: domain.RecoveryCase{ID: "c", Version: 4, LeakType: domain.FailedSubscription, AmountAtRiskMinor: 800000, RecoveryDeadline: now.Add(time.Hour)}, Diagnosis: recoverycontext.Diagnosis{Confidence: .9}, MerchantContext: recoverycontext.MerchantContext{RecoveryObjective: "MAXIMIZE_NET_RECOVERY", AllowedActions: []domain.ActionType{domain.ActionRetryLater}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5}, PaymentState: recoverycontext.PaymentState{MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID"}, TimingContext: recoverycontext.TimingContext{EvaluatedAt: now}}
	predictor := &predictorStub{}
	store := &storeStub{}
	result, err := NewService(contextStub{ctx}, predictor, store).Decide(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	if predictor.naturalCalls != 1 {
		t.Fatalf("natural calls %d", predictor.naturalCalls)
	}
	for _, c := range result.Decision.Optimization.Candidates {
		if c.NaturalRecoveryProbability != .35 {
			t.Fatalf("natural mismatch %+v", c)
		}
	}
	if store.snapshot.Decision.ID == "" || result.Policy.Decision != "APPROVE" {
		t.Fatalf("%+v", result)
	}
	if store.snapshot.Context.FeatureVersion != "recovery-context-v1" || len(store.snapshot.Eligibility.EligibleActions) == 0 {
		t.Fatal("decision explanation snapshot was not persisted")
	}
}

func TestObjectiveComparisonIsExplanationOnly(t *testing.T) {
	now := time.Now()
	decisionContext := recoverycontext.RecoveryDecisionContext{FeatureVersion: "recovery-context-v1", Case: domain.RecoveryCase{ID: "c", Version: 1, LeakType: domain.FailedSubscription, AmountAtRiskMinor: 100000, RecoveryDeadline: now.Add(time.Hour)}, Diagnosis: recoverycontext.Diagnosis{Confidence: .9}, MerchantContext: recoverycontext.MerchantContext{AllowedActions: []domain.ActionType{domain.ActionRetryLater}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5}, PaymentState: recoverycontext.PaymentState{MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID"}, TimingContext: recoverycontext.TimingContext{EvaluatedAt: now}}
	store := &storeStub{}
	rows, err := NewService(contextStub{decisionContext}, &predictorStub{}, store).CompareObjectives(context.Background(), "c")
	if err != nil || len(rows) != 5 {
		t.Fatalf("comparison: %d %v", len(rows), err)
	}
	if store.snapshot.Decision.ID != "" {
		t.Fatal("explanation-only comparison persisted a decision")
	}
}

func TestControlOnlyActionsCannotEnterExecutableOptimization(t *testing.T) {
	for _, action := range []domain.ActionType{domain.ActionWaitForPromiseToPay, domain.ActionRetention} {
		if scoreable[action] {
			t.Fatalf("control-only action is scoreable without an executor: %s", action)
		}
	}
}
