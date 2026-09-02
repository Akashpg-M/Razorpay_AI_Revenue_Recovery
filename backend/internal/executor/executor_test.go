package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/integrations/razorpay"
)

type memoryEmailStore struct {
	references map[string]string
	writes     int
}

func (s *memoryEmailStore) CaptureEmail(_ context.Context, r Request, _ string, _ json.RawMessage) (string, bool, error) {
	if reference, ok := s.references[r.IdempotencyKey]; ok {
		return reference, false, nil
	}
	s.writes++
	reference := "mail-1"
	s.references[r.IdempotencyKey] = reference
	return reference, true, nil
}

func TestEmailExecutorIsIdempotentAtDeliveryBoundary(t *testing.T) {
	store := &memoryEmailStore{references: map[string]string{}}
	executor := NewEmailExecutor(store)
	request := Request{ExecutionID: "execution-1", ScheduledActionID: "scheduled-1", Action: domain.ActionSendReminder, IdempotencyKey: "stable-key", AmountMinor: 12345, Currency: "INR", Parameters: json.RawMessage(`{"merchant_name":"Example","secret":"must-not-propagate"}`)}
	first, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProviderReference != second.ProviderReference || store.writes != 1 {
		t.Fatalf("expected one durable side effect, got writes=%d", store.writes)
	}
}

type fakePaymentLinkCreator struct {
	calls int
	last  razorpay.PaymentLinkRequest
}

func (f *fakePaymentLinkCreator) Execute(_ context.Context, actionID string, input razorpay.PaymentLinkRequest) (razorpay.PaymentLink, error) {
	f.calls++
	f.last = input
	if actionID == "" || input.Amount != 5000 {
		return razorpay.PaymentLink{}, errors.New("bad request")
	}
	return razorpay.PaymentLink{ID: "plink-1"}, nil
}
func (f *fakePaymentLinkCreator) Reconcile(context.Context, string) (razorpay.PaymentLink, error) {
	return razorpay.PaymentLink{ID: "plink-1", Status: "created"}, nil
}

func TestPaymentLinkExecutorUsesDurableRecoveryActionReference(t *testing.T) {
	creator := &fakePaymentLinkCreator{}
	result, err := NewPaymentLinkExecutor(creator).Execute(context.Background(), Request{ExecutionID: "e", ScheduledActionID: "s", RecoveryActionID: "a", CaseID: "case-1", MerchantID: "merchant-1", CustomerID: "customer-1", Action: domain.ActionSendPaymentLink, IdempotencyKey: "k", AmountMinor: 5000, Currency: "INR"})
	if err != nil || result.ProviderReference != "plink-1" || creator.calls != 1 {
		t.Fatalf("unexpected result: %+v err=%v", result, err)
	}
	if creator.last.ReferenceID != "s" || creator.last.Notes["recovery_case_id"] != "case-1" || creator.last.Notes["recovery_action_id"] != "a" || creator.last.Notes["merchant_id"] != "merchant-1" || creator.last.Notes["customer_id"] != "customer-1" {
		t.Fatalf("payment link correlation metadata=%+v", creator.last)
	}
}

func TestRetryExecutorDoesNotClaimUnsupportedProviderCapability(t *testing.T) {
	result, err := NewRetryExecutor(nil).Execute(context.Background(), Request{ExecutionID: "e", Action: domain.ActionRetryNow, IdempotencyKey: "k"})
	if err == nil || result.Retryable || result.FailureClass != "PROVIDER_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("unexpected capability result: %+v err=%v", result, err)
	}
}

type timeoutCreator struct{}

func (timeoutCreator) Execute(context.Context, string, razorpay.PaymentLinkRequest) (razorpay.PaymentLink, error) {
	return razorpay.PaymentLink{}, context.DeadlineExceeded
}
func (timeoutCreator) Reconcile(context.Context, string) (razorpay.PaymentLink, error) {
	return razorpay.PaymentLink{}, context.DeadlineExceeded
}

func TestLocalPaymentLinkExecutorIsDeterministicAndExternalFree(t *testing.T) {
	store := &memoryEmailStore{references: map[string]string{}}
	local := NewLocalPaymentLinkExecutor(store)
	request := Request{ExecutionID: "e", ScheduledActionID: "s", RecoveryActionID: "a", Action: domain.ActionSendPaymentLink, IdempotencyKey: "local-key", AmountMinor: 100, Currency: "INR"}
	first, err := local.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := local.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != "local-payment-link" || first.ProviderReference != second.ProviderReference || store.writes != 1 {
		t.Fatalf("first=%+v second=%+v writes=%d", first, second, store.writes)
	}
}
func TestProviderTimeoutIsRetryableAndClassified(t *testing.T) {
	result, err := NewPaymentLinkExecutor(timeoutCreator{}).Execute(context.Background(), Request{Action: domain.ActionSendPaymentLink})
	if err == nil || !result.Retryable || result.FailureClass != "TIMEOUT" {
		t.Fatalf("unexpected timeout: %+v %v", result, err)
	}
}
