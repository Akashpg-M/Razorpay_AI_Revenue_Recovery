package intelligence

import (
	"context"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

type contexts struct {
	value recoverycontext.RecoveryDecisionContext
}

func (c contexts) Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error) {
	return c.value, nil
}

type predictor struct{ received []string }

func (p *predictor) PredictOutcomes(_ context.Context, r decisionclient.PredictionRequest) (decisionclient.PredictionResponse, error) {
	p.received = r.EligibleActions
	items := []decisionclient.OutcomePrediction{}
	for _, a := range r.EligibleActions {
		prob := .4
		if a == "RETRY_LATER" {
			prob = .6
		}
		items = append(items, decisionclient.OutcomePrediction{Action: a, RecoveryProbability: prob, ModelVersion: "outcome-v1", FeatureVersion: "features-v1", PredictionTimestamp: time.Now()})
	}
	return decisionclient.PredictionResponse{Predictions: items}, nil
}

type predictionStore struct{ saved []domain.ActionPrediction }

func (s *predictionStore) SaveActionPredictions(_ context.Context, p []domain.ActionPrediction) error {
	s.saved = p
	return nil
}
func TestPipelinePredictsOnlyEligibleScoreableActionsAndPersists(t *testing.T) {
	now := time.Now()
	ctx := recoverycontext.RecoveryDecisionContext{Case: domain.RecoveryCase{ID: "case", LeakType: domain.FailedSubscription, AmountAtRiskMinor: 10000, RecoveryDeadline: now.Add(time.Hour)}, TimingContext: recoverycontext.TimingContext{EvaluatedAt: now}, CustomerProfile: recoverycontext.CustomerProfile{OptedOut: true}, MerchantContext: recoverycontext.MerchantContext{AllowedActions: []domain.ActionType{domain.ActionRetryLater, domain.ActionSendReminder}, AllowedChannels: []string{"EMAIL"}, MaxRetries: 3, MaxContactsPerDay: 3, MaxContactsPerWeek: 5}, PaymentState: recoverycontext.PaymentState{MandateStatus: "ACTIVE", PaymentMethodStatus: "VALID", AvailableChannels: []string{"EMAIL"}}}
	p := &predictor{}
	store := &predictionStore{}
	result, err := NewService(contexts{ctx}, p, store).Predict(context.Background(), "case")
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range p.received {
		if action == "SEND_REMINDER" || action == "STOP" || action == "ESCALATE_TO_HUMAN" {
			t.Fatalf("invalid/control action reached model: %s", action)
		}
	}
	if len(store.saved) != len(p.received) || len(result.Predictions) == 0 {
		t.Fatalf("saved=%d predicted=%d", len(store.saved), len(p.received))
	}
}
