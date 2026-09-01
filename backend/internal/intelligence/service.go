package intelligence

import (
	"context"
	"fmt"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/eligibility"
	"revenue-recovery/backend/internal/id"
	"time"
)

type ContextProvider interface {
	Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error)
}
type Predictor interface {
	PredictOutcomes(context.Context, decisionclient.PredictionRequest) (decisionclient.PredictionResponse, error)
}
type PredictionStore interface {
	SaveActionPredictions(context.Context, []domain.ActionPrediction) error
}
type Service struct {
	contexts  ContextProvider
	predictor Predictor
	store     PredictionStore
	now       func() time.Time
}

func NewService(contexts ContextProvider, predictor Predictor, store PredictionStore) *Service {
	return &Service{contexts: contexts, predictor: predictor, store: store, now: time.Now}
}

type Result struct {
	Eligibility eligibility.Result        `json:"eligibility"`
	Predictions []domain.ActionPrediction `json:"predictions"`
}

var scoreable = map[domain.ActionType]bool{domain.ActionWait: true, domain.ActionRetryNow: true, domain.ActionRetryLater: true, domain.ActionSendReminder: true, domain.ActionSendPaymentLink: true, domain.ActionSendCheckoutRecoveryLink: true, domain.ActionRequestPaymentMethodUpdate: true, domain.ActionSuggestAlternateMethod: true, domain.ActionWaitForPromiseToPay: true, domain.ActionRetention: true}

func (s *Service) Predict(ctx context.Context, caseID domain.ID) (Result, error) {
	decisionContext, err := s.contexts.Get(ctx, caseID)
	if err != nil {
		return Result{}, err
	}
	eligible := eligibility.Evaluate(decisionContext)
	actions := []string{}
	for _, action := range eligible.EligibleActions {
		if scoreable[action] {
			actions = append(actions, string(action))
		}
	}
	if len(actions) == 0 {
		return Result{Eligibility: eligible}, fmt.Errorf("no scoreable eligible actions")
	}
	response, err := s.predictor.PredictOutcomes(ctx, decisionclient.PredictionRequest{Context: decisionContext, EligibleActions: actions})
	if err != nil {
		return Result{}, err
	}
	natural := 0.0
	for _, prediction := range response.Predictions {
		if prediction.Action == string(domain.ActionWait) {
			natural = prediction.RecoveryProbability
		}
	}
	predictions := make([]domain.ActionPrediction, 0, len(response.Predictions))
	for _, prediction := range response.Predictions {
		uplift := prediction.RecoveryProbability - natural
		predictions = append(predictions, domain.ActionPrediction{ID: domain.ID(id.New()), CaseID: caseID, ActionType: domain.ActionType(prediction.Action), RecoveryProbability: prediction.RecoveryProbability, NaturalRecoveryProbability: natural, IncrementalUplift: uplift, ExpectedNetValueMinor: int64(uplift * float64(decisionContext.Case.AmountAtRiskMinor)), FeatureVersion: prediction.FeatureVersion, ModelVersion: prediction.ModelVersion, Explanation: []byte(`{}`), CreatedAt: prediction.PredictionTimestamp})
	}
	if err = s.store.SaveActionPredictions(ctx, predictions); err != nil {
		return Result{}, err
	}
	return Result{Eligibility: eligible, Predictions: predictions}, nil
}
