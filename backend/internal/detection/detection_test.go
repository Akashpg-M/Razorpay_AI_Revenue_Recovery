package detection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/recovery"
)

type caseRepo struct {
	cases  map[domain.ID]domain.RecoveryCase
	events map[domain.ID][]domain.RecoveryEvent
}

func newCaseRepo() *caseRepo {
	return &caseRepo{cases: map[domain.ID]domain.RecoveryCase{}, events: map[domain.ID][]domain.RecoveryEvent{}}
}
func (r *caseRepo) CreateCase(_ context.Context, c domain.RecoveryCase, e []domain.RecoveryEvent) error {
	r.cases[c.ID] = c
	r.events[c.ID] = e
	return nil
}
func (r *caseRepo) GetCase(_ context.Context, id domain.ID) (domain.RecoveryCase, error) {
	c, ok := r.cases[id]
	if !ok {
		return c, recovery.ErrNotFound
	}
	return c, nil
}
func (r *caseRepo) GetCaseBySource(_ context.Context, m domain.ID, s string) (domain.RecoveryCase, error) {
	for _, c := range r.cases {
		if c.MerchantID == m && c.SourceReference == s {
			return c, nil
		}
	}
	return domain.RecoveryCase{}, recovery.ErrNotFound
}
func (r *caseRepo) TransitionCase(context.Context, domain.ID, int64, domain.CaseState, domain.RecoveryEvent) (domain.RecoveryCase, error) {
	panic("unused")
}
func (r *caseRepo) AppendEvent(context.Context, domain.RecoveryEvent) (domain.RecoveryEvent, error) {
	panic("unused")
}
func (r *caseRepo) ListEvents(_ context.Context, id domain.ID) ([]domain.RecoveryEvent, error) {
	return r.events[id], nil
}

type checkoutStore struct{ sessions map[string]CheckoutSession }

func (s *checkoutStore) UpsertCheckout(_ context.Context, c CheckoutSession) (CheckoutSession, error) {
	s.sessions[c.CheckoutID] = c
	return c, nil
}
func (s *checkoutStore) GetCheckout(_ context.Context, id string) (CheckoutSession, error) {
	c, ok := s.sessions[id]
	if !ok {
		return c, recovery.ErrNotFound
	}
	return c, nil
}

func TestSubscriptionEventCreatesCanonicalIdempotentCase(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(SubscriptionEvent{EventType: "payment.failed", MerchantID: "m1", CustomerID: "c1", AmountMinor: 849900,
		Currency: "INR", PaymentID: "pay_1", SubscriptionID: "sub_1", FailureCode: "insufficient_funds", OccurredAt: now})
	repo := newCaseRepo()
	service := NewService(recovery.NewService(repo))
	adapter := SubscriptionAdapter{Provider: "razorpay"}
	first, err := service.Detect(context.Background(), adapter, payload, "evt_1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Detect(context.Background(), adapter, payload, "evt_1")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || second.Created || first.Case.ID != second.Case.ID {
		t.Fatalf("idempotency failed: %+v %+v", first, second)
	}
	if first.Case.LeakType != domain.FailedSubscription {
		t.Fatalf("got %s", first.Case.LeakType)
	}
}

func TestCheckoutStateProducesSameCanonicalContract(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	store := &checkoutStore{sessions: map[string]CheckoutSession{}}
	adapter := CheckoutAdapter{Store: store}
	started, _ := json.Marshal(CheckoutEvent{EventType: "CHECKOUT_STARTED", CheckoutID: "co_1", MerchantID: "m1", CustomerID: "c1",
		AmountMinor: 299900, Currency: "INR", CheckoutStage: "PAYMENT", OccurredAt: now, ValidUntil: now.Add(24 * time.Hour)})
	if leak, err := adapter.Normalize(context.Background(), started, "evt_start"); err != nil || leak != nil {
		t.Fatalf("start: leak=%v err=%v", leak, err)
	}
	selected, _ := json.Marshal(CheckoutEvent{EventType: "CHECKOUT_PAYMENT_SELECTED", CheckoutID: "co_1", PaymentMethod: "UPI", OccurredAt: now.Add(time.Minute)})
	if _, err := adapter.Normalize(context.Background(), selected, "evt_selected"); err != nil {
		t.Fatal(err)
	}
	abandoned, _ := json.Marshal(CheckoutEvent{EventType: "CHECKOUT_ABANDONED", CheckoutID: "co_1", OccurredAt: now.Add(time.Hour)})
	leak, err := adapter.Normalize(context.Background(), abandoned, "evt_abandoned")
	if err != nil {
		t.Fatal(err)
	}
	if leak.LeakType != domain.CheckoutAbandonment || leak.SourceReference != "co_1" {
		t.Fatalf("unexpected leak %+v", leak)
	}
}

func TestMalformedEventsRejected(t *testing.T) {
	adapter := SubscriptionAdapter{}
	if _, err := adapter.Normalize(context.Background(), json.RawMessage(`{"event_type":"payment.failed"}`), "evt"); err == nil {
		t.Fatal("expected malformed event rejection")
	}
	if _, err := adapter.Normalize(context.Background(), json.RawMessage(`not-json`), "evt"); err == nil {
		t.Fatal("expected invalid JSON rejection")
	}
}
