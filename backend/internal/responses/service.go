package responses

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
)

type Type string

const (
	Acknowledgement    Type = "ACKNOWLEDGEMENT"
	IntentToPay        Type = "INTENT_TO_PAY"
	PromiseToPay       Type = "PROMISE_TO_PAY"
	OptOut             Type = "OPT_OUT"
	PaymentMethodIssue Type = "PAYMENT_METHOD_ISSUE"
	Unresolved         Type = "UNRESOLVED"
)

var validTypes = map[Type]bool{Acknowledgement: true, IntentToPay: true, PromiseToPay: true, OptOut: true, PaymentMethodIssue: true, Unresolved: true}

type Response struct {
	ID            domain.ID       `json:"response_id"`
	CaseID        domain.ID       `json:"case_id"`
	Type          Type            `json:"response_type"`
	Payload       json.RawMessage `json:"payload"`
	Source        string          `json:"source"`
	ReceivedAt    time.Time       `json:"received_at"`
	CorrelationID string          `json:"correlation_id"`
}

type Store interface {
	SaveCustomerResponse(context.Context, Response) (bool, error)
}

// PromiseCreator keeps response persistence independent from promise parsing while
// allowing a newly persisted PROMISE_TO_PAY response to create exactly one promise.
type PromiseCreator interface {
	CreateFromResponse(context.Context, Response) error
}

type Service struct {
	store          Store
	promiseCreator PromiseCreator
	now            func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) SetPromiseCreator(creator PromiseCreator) { s.promiseCreator = creator }

func (s *Service) Ingest(ctx context.Context, response Response) (Response, bool, error) {
	if !validTypes[response.Type] {
		return Response{}, false, errors.New("invalid customer response type")
	}
	if response.CaseID == "" || response.Source == "" || response.CorrelationID == "" {
		return Response{}, false, errors.New("case_id, source, and correlation_id are required")
	}
	if len(response.Payload) == 0 {
		response.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(response.Payload) {
		return Response{}, false, errors.New("payload must be valid JSON")
	}
	if response.ID == "" {
		response.ID = domain.ID(id.New())
	}
	if response.ReceivedAt.IsZero() {
		response.ReceivedAt = s.now().UTC()
	}
	created, err := s.store.SaveCustomerResponse(ctx, response)
	if err != nil || !created || response.Type != PromiseToPay || s.promiseCreator == nil {
		return response, created, err
	}
	if err = s.promiseCreator.CreateFromResponse(ctx, response); err != nil {
		return response, created, err
	}
	return response, created, nil
}
