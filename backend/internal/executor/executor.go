package executor

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"time"
)

type Request struct {
	ExecutionID        domain.ID
	ScheduledActionID  domain.ID
	Action             domain.ActionType
	IdempotencyKey     string
	AmountMinor        int64
	Currency           string
	RecipientReference string
	Parameters         json.RawMessage
}
type Result struct {
	ExecutionID       domain.ID         `json:"action_execution_id"`
	Action            domain.ActionType `json:"action"`
	Status            string            `json:"status"`
	Provider          string            `json:"provider"`
	ProviderReference string            `json:"provider_reference"`
	IdempotencyKey    string            `json:"idempotency_key"`
	ExecutedAt        time.Time         `json:"executed_at"`
	FailureClass      string            `json:"failure_class,omitempty"`
	Retryable         bool              `json:"retryable"`
}
type Executor interface {
	Supports(domain.ActionType) bool
	Execute(context.Context, Request) (Result, error)
	Reconcile(context.Context, string) (Result, error)
}
type Registry struct{ executors []Executor }

func NewRegistry(executors ...Executor) *Registry { return &Registry{executors: executors} }
func (r *Registry) Execute(ctx context.Context, request Request) (Result, error) {
	for _, candidate := range r.executors {
		if candidate.Supports(request.Action) {
			return candidate.Execute(ctx, request)
		}
	}
	return Result{ExecutionID: request.ExecutionID, Action: request.Action, Status: "FAILED", IdempotencyKey: request.IdempotencyKey, FailureClass: "UNSUPPORTED_ACTION", Retryable: false}, errors.New("no executor supports action")
}
func (r *Registry) Reconcile(ctx context.Context, action domain.ActionType, idempotencyKey string) (Result, error) {
	for _, candidate := range r.executors {
		if candidate.Supports(action) {
			return candidate.Reconcile(ctx, idempotencyKey)
		}
	}
	return Result{}, errors.New("no executor supports reconciliation")
}

type EmailDeliveryStore interface {
	CaptureEmail(context.Context, Request, string, json.RawMessage) (string, bool, error)
}
type EmailExecutor struct {
	store EmailDeliveryStore
	now   func() time.Time
}

func NewEmailExecutor(store EmailDeliveryStore) *EmailExecutor {
	return &EmailExecutor{store: store, now: time.Now}
}
func (e *EmailExecutor) Supports(a domain.ActionType) bool {
	switch a {
	case domain.ActionSendReminder, domain.ActionRequestPaymentMethodUpdate, domain.ActionSendCheckoutRecoveryLink, domain.ActionSuggestAlternateMethod:
		return true
	}
	return false
}
func (e *EmailExecutor) Execute(ctx context.Context, r Request) (Result, error) {
	template := map[domain.ActionType]string{domain.ActionSendReminder: "failed-payment-reminder", domain.ActionRequestPaymentMethodUpdate: "payment-method-update", domain.ActionSendCheckoutRecoveryLink: "checkout-recovery", domain.ActionSuggestAlternateMethod: "alternate-payment-method"}[r.Action]
	safe := json.RawMessage(`{"amount_minor":0}`)
	var supplied map[string]any
	if json.Unmarshal(r.Parameters, &supplied) == nil {
		allowed := map[string]any{"amount_minor": r.AmountMinor, "currency": r.Currency}
		for _, key := range []string{"merchant_name", "recovery_url", "deadline"} {
			if value, ok := supplied[key]; ok {
				allowed[key] = value
			}
		}
		safe, _ = json.Marshal(allowed)
	}
	reference, _, err := e.store.CaptureEmail(ctx, r, template, safe)
	if err != nil {
		class, retryable := classify(err)
		return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "FAILED", IdempotencyKey: r.IdempotencyKey, FailureClass: class, Retryable: retryable}, err
	}
	return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "SUCCEEDED", Provider: "local-email-capture", ProviderReference: reference, IdempotencyKey: r.IdempotencyKey, ExecutedAt: e.now().UTC()}, nil
}
func (e *EmailExecutor) Reconcile(context.Context, string) (Result, error) {
	return Result{}, errors.New("local email capture does not require reconciliation")
}

type PaymentLinkCreator interface {
	Execute(context.Context, string, razorpay.PaymentLinkRequest) (razorpay.PaymentLink, error)
}
type PaymentLinkExecutor struct {
	creator PaymentLinkCreator
	now     func() time.Time
}

func NewPaymentLinkExecutor(creator PaymentLinkCreator) *PaymentLinkExecutor {
	return &PaymentLinkExecutor{creator: creator, now: time.Now}
}
func (e *PaymentLinkExecutor) Supports(a domain.ActionType) bool {
	return a == domain.ActionSendPaymentLink
}
func (e *PaymentLinkExecutor) Execute(ctx context.Context, r Request) (Result, error) {
	link, err := e.creator.Execute(ctx, string(r.ScheduledActionID), razorpay.PaymentLinkRequest{Amount: r.AmountMinor, Currency: r.Currency, ReferenceID: string(r.ScheduledActionID), Description: "Merchant-approved revenue recovery"})
	if err != nil {
		class, retryable := classify(err)
		return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "FAILED", IdempotencyKey: r.IdempotencyKey, FailureClass: class, Retryable: retryable}, err
	}
	return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "SUCCEEDED", Provider: "razorpay", ProviderReference: link.ID, IdempotencyKey: r.IdempotencyKey, ExecutedAt: e.now().UTC()}, nil
}

func classify(err error) (string, bool) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT", true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "TIMEOUT", true
	}
	var providerError *razorpay.APIError
	if errors.As(err, &providerError) && providerError.StatusCode >= 400 && providerError.StatusCode < 500 && providerError.StatusCode != 408 && providerError.StatusCode != 429 {
		return "PERMANENT_PROVIDER_ERROR", false
	}
	return "TRANSIENT_PROVIDER_ERROR", true
}
func (e *PaymentLinkExecutor) Reconcile(context.Context, string) (Result, error) {
	return Result{}, errors.New("reconciliation uses payment status adapter")
}

type RetryProvider interface {
	RequestRetry(context.Context, string, json.RawMessage) (string, error)
}
type namedRetryProvider interface{ ProviderName() string }
type RetryExecutor struct {
	provider RetryProvider
	now      func() time.Time
}

func NewRetryExecutor(provider RetryProvider) *RetryExecutor {
	return &RetryExecutor{provider: provider, now: time.Now}
}
func (e *RetryExecutor) Supports(a domain.ActionType) bool {
	return a == domain.ActionRetryNow || a == domain.ActionRetryLater
}
func (e *RetryExecutor) Execute(ctx context.Context, r Request) (Result, error) {
	if e.provider == nil {
		return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "FAILED", IdempotencyKey: r.IdempotencyKey, FailureClass: "PROVIDER_CAPABILITY_UNAVAILABLE", Retryable: false}, errors.New("direct Razorpay retry API is not configured or claimed")
	}
	reference, err := e.provider.RequestRetry(ctx, r.IdempotencyKey, r.Parameters)
	if err != nil {
		return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "FAILED", IdempotencyKey: r.IdempotencyKey, FailureClass: "TRANSIENT_PROVIDER_ERROR", Retryable: true}, err
	}
	provider := "configured-retry-provider"
	if named, ok := e.provider.(namedRetryProvider); ok {
		provider = named.ProviderName()
	}
	return Result{ExecutionID: r.ExecutionID, Action: r.Action, Status: "OUTCOME_PENDING", Provider: provider, ProviderReference: reference, IdempotencyKey: r.IdempotencyKey, ExecutedAt: e.now().UTC()}, nil
}
func (e *RetryExecutor) Reconcile(context.Context, string) (Result, error) {
	return Result{}, errors.New("retry provider reconciliation not configured")
}
