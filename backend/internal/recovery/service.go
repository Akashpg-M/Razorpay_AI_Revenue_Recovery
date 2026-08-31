package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
)

var (
	ErrNotFound = errors.New("recovery case not found")
	ErrConflict = errors.New("recovery case version conflict")
)

type Repository interface {
	CreateCase(context.Context, domain.RecoveryCase, []domain.RecoveryEvent) error
	GetCase(context.Context, domain.ID) (domain.RecoveryCase, error)
	TransitionCase(context.Context, domain.ID, int64, domain.CaseState, domain.RecoveryEvent) (domain.RecoveryCase, error)
	AppendEvent(context.Context, domain.RecoveryEvent) (domain.RecoveryEvent, error)
	ListEvents(context.Context, domain.ID) ([]domain.RecoveryEvent, error)
}

type Clock func() time.Time

type Service struct {
	repository Repository
	now        Clock
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

type CreateCaseInput struct {
	LeakType                domain.LeakType `json:"leak_type"`
	MerchantID              domain.ID       `json:"merchant_id"`
	CustomerID              domain.ID       `json:"customer_id"`
	AmountAtRiskMinor       int64           `json:"amount_at_risk_minor"`
	Currency                string          `json:"currency"`
	SourceReference         string          `json:"source_reference"`
	SourceStatus            string          `json:"source_status"`
	FailureOrLeakContext    json.RawMessage `json:"failure_or_leak_context"`
	CustomerContextSnapshot json.RawMessage `json:"customer_context_snapshot"`
	MerchantPolicySnapshot  json.RawMessage `json:"merchant_policy_snapshot"`
	RecoveryDeadline        time.Time       `json:"recovery_deadline"`
	Actor                   domain.Actor    `json:"actor"`
	CorrelationID           string          `json:"correlation_id"`
}

func (s *Service) CreateCase(ctx context.Context, input CreateCaseInput) (domain.RecoveryCase, error) {
	if input.LeakType != domain.FailedSubscription && input.LeakType != domain.CheckoutAbandonment {
		return domain.RecoveryCase{}, fmt.Errorf("invalid leak_type %q", input.LeakType)
	}
	if input.MerchantID == "" || input.CustomerID == "" || strings.TrimSpace(input.SourceReference) == "" {
		return domain.RecoveryCase{}, errors.New("merchant_id, customer_id and source_reference are required")
	}
	if input.AmountAtRiskMinor <= 0 || len(input.Currency) != 3 {
		return domain.RecoveryCase{}, errors.New("amount_at_risk_minor must be positive and currency must be a 3-letter code")
	}
	now := s.now().UTC()
	if !input.RecoveryDeadline.After(now) {
		return domain.RecoveryCase{}, errors.New("recovery_deadline must be in the future")
	}
	actor := normalizeActor(input.Actor)
	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = id.New()
	}
	caseID := domain.ID(id.New())
	recoveryCase := domain.RecoveryCase{
		ID: caseID, LeakType: input.LeakType, MerchantID: input.MerchantID,
		CustomerID: input.CustomerID, AmountAtRiskMinor: input.AmountAtRiskMinor,
		Currency: strings.ToUpper(input.Currency), SourceReference: input.SourceReference,
		SourceStatus: input.SourceStatus, FailureOrLeakContext: jsonObject(input.FailureOrLeakContext),
		CustomerContextSnapshot: jsonObject(input.CustomerContextSnapshot),
		MerchantPolicySnapshot:  jsonObject(input.MerchantPolicySnapshot), CurrentState: domain.StateDetected,
		RecoveryDeadline: input.RecoveryDeadline.UTC(), AttributionStatus: "PENDING",
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	basePayload, _ := json.Marshal(map[string]any{
		"leak_type": input.LeakType, "amount_at_risk_minor": input.AmountAtRiskMinor,
		"currency": recoveryCase.Currency, "source_reference": input.SourceReference,
	})
	events := []domain.RecoveryEvent{
		newEvent(caseID, domain.EventRevenueRiskDetected, actor, basePayload, correlationID, now),
		newEvent(caseID, domain.EventCaseCreated, actor, json.RawMessage(`{"state":"DETECTED"}`), correlationID, now),
	}
	if err := s.repository.CreateCase(ctx, recoveryCase, events); err != nil {
		return domain.RecoveryCase{}, err
	}
	return recoveryCase, nil
}

type TransitionInput struct {
	ToState         domain.CaseState `json:"to_state"`
	ExpectedVersion int64            `json:"expected_version"`
	Actor           domain.Actor     `json:"actor"`
	Payload         json.RawMessage  `json:"payload"`
	ModelVersion    string           `json:"model_version"`
	CorrelationID   string           `json:"correlation_id"`
}

func (s *Service) Transition(ctx context.Context, caseID domain.ID, input TransitionInput) (domain.RecoveryCase, error) {
	current, err := s.repository.GetCase(ctx, caseID)
	if err != nil {
		return domain.RecoveryCase{}, err
	}
	if input.ExpectedVersion != 0 && input.ExpectedVersion != current.Version {
		return domain.RecoveryCase{}, ErrConflict
	}
	if err := domain.ValidateTransition(current.CurrentState, input.ToState); err != nil {
		return domain.RecoveryCase{}, err
	}
	correlationID := input.CorrelationID
	if correlationID == "" {
		correlationID = id.New()
	}
	payload := mergeTransitionPayload(input.Payload, current.CurrentState, input.ToState, current.Version)
	event := newEvent(caseID, domain.EventForTransition(input.ToState), normalizeActor(input.Actor), payload, correlationID, s.now().UTC())
	event.ModelVersion = input.ModelVersion
	return s.repository.TransitionCase(ctx, caseID, current.Version, input.ToState, event)
}

type RecordEventInput struct {
	Type          domain.EventType `json:"event_type"`
	Actor         domain.Actor     `json:"actor"`
	Payload       json.RawMessage  `json:"payload"`
	ModelVersion  string           `json:"model_version"`
	CorrelationID string           `json:"correlation_id"`
}

func (s *Service) RecordEvent(ctx context.Context, caseID domain.ID, input RecordEventInput) (domain.RecoveryEvent, error) {
	if !input.Type.IsValid() || input.Type == domain.EventCaseCreated || input.Type == domain.EventStateTransitioned {
		return domain.RecoveryEvent{}, fmt.Errorf("invalid manually recorded event type %q", input.Type)
	}
	if _, err := s.repository.GetCase(ctx, caseID); err != nil {
		return domain.RecoveryEvent{}, err
	}
	if input.CorrelationID == "" {
		input.CorrelationID = id.New()
	}
	event := newEvent(caseID, input.Type, normalizeActor(input.Actor), jsonObject(input.Payload), input.CorrelationID, s.now().UTC())
	event.ModelVersion = input.ModelVersion
	return s.repository.AppendEvent(ctx, event)
}

func (s *Service) GetCase(ctx context.Context, caseID domain.ID) (domain.RecoveryCase, error) {
	return s.repository.GetCase(ctx, caseID)
}

func (s *Service) Events(ctx context.Context, caseID domain.ID) ([]domain.RecoveryEvent, error) {
	if _, err := s.repository.GetCase(ctx, caseID); err != nil {
		return nil, err
	}
	return s.repository.ListEvents(ctx, caseID)
}

func newEvent(caseID domain.ID, eventType domain.EventType, actor domain.Actor, payload json.RawMessage, correlationID string, now time.Time) domain.RecoveryEvent {
	return domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: caseID, Type: eventType, Timestamp: now,
		Actor: actor, Payload: jsonObject(payload), CorrelationID: correlationID}
}

func normalizeActor(actor domain.Actor) domain.Actor {
	if actor.Type == "" {
		actor.Type = "SYSTEM"
	}
	if actor.ID == "" {
		actor.ID = "recovery-engine"
	}
	return actor
}

func jsonObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}

func mergeTransitionPayload(extra json.RawMessage, from, to domain.CaseState, version int64) json.RawMessage {
	payload := map[string]any{"from_state": from, "to_state": to, "previous_version": version}
	if len(extra) > 0 {
		var supplied map[string]any
		if json.Unmarshal(extra, &supplied) == nil {
			for key, value := range supplied {
				payload[key] = value
			}
		}
	}
	result, _ := json.Marshal(payload)
	return result
}
