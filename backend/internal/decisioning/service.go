package decisioning

import (
	"context"
	"errors"
	recoverycontext "revenue-recovery/backend/internal/context"
	"revenue-recovery/backend/internal/decisionclient"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/economicgate"
	"revenue-recovery/backend/internal/eligibility"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/optimizer"
	"revenue-recovery/backend/internal/policy"
	"time"
)

type Predictor interface {
	PredictOutcomes(context.Context, decisionclient.PredictionRequest) (decisionclient.PredictionResponse, error)
	PredictNatural(context.Context, any) (decisionclient.NaturalPredictionResponse, error)
}
type ContextProvider interface {
	Get(context.Context, domain.ID) (recoverycontext.RecoveryDecisionContext, error)
}
type Store interface {
	SaveDecision(context.Context, Snapshot) error
}
type Service struct {
	contexts  ContextProvider
	predictor Predictor
	store     Store
	costs     optimizer.CostModel
	now       func() time.Time
}

func NewService(contexts ContextProvider, predictor Predictor, store Store) *Service {
	return &Service{contexts: contexts, predictor: predictor, store: store, costs: optimizer.DefaultCostModel(), now: time.Now}
}

type NaturalSnapshot struct {
	ID             domain.ID `json:"prediction_id"`
	CaseID         domain.ID `json:"case_id"`
	CaseVersion    int64     `json:"case_version"`
	ContextVersion string    `json:"context_version"`
	Probability    float64   `json:"natural_recovery_probability"`
	ModelVersion   string    `json:"model_version"`
	FeatureVersion string    `json:"feature_version"`
	PredictedAt    time.Time `json:"predicted_at"`
}
type Decision struct {
	ID                  domain.ID        `json:"decision_id"`
	CaseID              domain.ID        `json:"case_id"`
	CaseVersion         int64            `json:"case_version"`
	ContextVersion      string           `json:"context_version"`
	OutcomeModelVersion string           `json:"outcome_model_version"`
	NaturalModelVersion string           `json:"natural_model_version"`
	Optimization        optimizer.Result `json:"optimization"`
}
type Snapshot struct {
	Decision    Decision            `json:"decision"`
	Natural     NaturalSnapshot     `json:"natural_recovery"`
	Eligibility eligibility.Result  `json:"eligibility"`
	Gate        economicgate.Result `json:"economic_gate"`
	Policy      policy.Result       `json:"policy"`
}

var scoreable = map[domain.ActionType]bool{domain.ActionRetryNow: true, domain.ActionRetryLater: true, domain.ActionSendReminder: true, domain.ActionSendPaymentLink: true, domain.ActionSendCheckoutRecoveryLink: true, domain.ActionRequestPaymentMethodUpdate: true, domain.ActionSuggestAlternateMethod: true, domain.ActionWaitForPromiseToPay: true, domain.ActionRetention: true}

func (s *Service) Decide(ctx context.Context, caseID domain.ID) (Snapshot, error) {
	decisionContext, err := s.contexts.Get(ctx, caseID)
	if err != nil {
		return Snapshot{}, err
	}
	eligible := eligibility.Evaluate(decisionContext)
	actions := []string{}
	for _, action := range eligible.EligibleActions {
		if scoreable[action] {
			actions = append(actions, string(action))
		}
	}
	natural, err := s.predictor.PredictNatural(ctx, decisionContext)
	if err != nil {
		return Snapshot{}, err
	}
	outcomes := decisionclient.PredictionResponse{}
	if len(actions) > 0 {
		outcomes, err = s.predictor.PredictOutcomes(ctx, decisionclient.PredictionRequest{Context: decisionContext, EligibleActions: actions})
		if err != nil {
			return Snapshot{}, err
		}
	}
	now := s.now().UTC()
	ranked := optimizer.Rank(decisionContext, outcomes.Predictions, natural.NaturalRecoveryProbability, s.costs, now)
	decisionID := domain.ID(id.New())
	decision := Decision{ID: decisionID, CaseID: caseID, CaseVersion: decisionContext.Case.Version, ContextVersion: decisionContext.FeatureVersion, OutcomeModelVersion: modelVersion(outcomes.Predictions), NaturalModelVersion: natural.ModelVersion, Optimization: ranked}
	naturalSnapshot := NaturalSnapshot{ID: domain.ID(id.New()), CaseID: caseID, CaseVersion: decisionContext.Case.Version, ContextVersion: decisionContext.FeatureVersion, Probability: natural.NaturalRecoveryProbability, ModelVersion: natural.ModelVersion, FeatureVersion: natural.FeatureVersion, PredictedAt: natural.PredictionTimestamp}
	gate := economicgate.Evaluate(decisionID, caseID, ranked.Selected, decisionContext.MerchantContext.MinimumEconomicValueMinor, now)
	gate.ID = domain.ID(id.New())
	policyResult := policy.Evaluate(decisionContext, decisionID, decisionContext.Case.Version, ranked.Selected, gate, now)
	policyResult.ID = domain.ID(id.New())
	policyResult.EconomicGateID = gate.ID
	snapshot := Snapshot{Decision: decision, Natural: naturalSnapshot, Eligibility: eligible, Gate: gate, Policy: policyResult}
	if err = s.store.SaveDecision(ctx, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}
func modelVersion(predictions []decisionclient.OutcomePrediction) string {
	if len(predictions) == 0 {
		return "outcome-v1"
	}
	return predictions[0].ModelVersion
}

var ErrStaleDecision = errors.New("decision is stale")
