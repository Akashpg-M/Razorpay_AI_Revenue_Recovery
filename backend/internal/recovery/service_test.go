package recovery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"revenue-recovery/backend/internal/domain"
)

type memoryRepository struct {
	cases  map[domain.ID]domain.RecoveryCase
	events map[domain.ID][]domain.RecoveryEvent
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{cases: map[domain.ID]domain.RecoveryCase{}, events: map[domain.ID][]domain.RecoveryEvent{}}
}
func (m *memoryRepository) CreateCase(_ context.Context, c domain.RecoveryCase, events []domain.RecoveryEvent) error {
	m.cases[c.ID] = c
	for i := range events {
		events[i].Sequence = int64(i + 1)
	}
	m.events[c.ID] = append([]domain.RecoveryEvent(nil), events...)
	return nil
}
func (m *memoryRepository) GetCase(_ context.Context, id domain.ID) (domain.RecoveryCase, error) {
	c, ok := m.cases[id]
	if !ok {
		return domain.RecoveryCase{}, ErrNotFound
	}
	return c, nil
}
func (m *memoryRepository) GetCaseBySource(_ context.Context, merchantID domain.ID, source string) (domain.RecoveryCase, error) {
	for _, c := range m.cases {
		if c.MerchantID == merchantID && c.SourceReference == source {
			return c, nil
		}
	}
	return domain.RecoveryCase{}, ErrNotFound
}
func (m *memoryRepository) TransitionCase(_ context.Context, id domain.ID, expected int64, to domain.CaseState, event domain.RecoveryEvent) (domain.RecoveryCase, error) {
	c := m.cases[id]
	if c.Version != expected {
		return domain.RecoveryCase{}, ErrConflict
	}
	c.CurrentState = to
	c.Version++
	c.UpdatedAt = event.Timestamp
	m.cases[id] = c
	event.Sequence = int64(len(m.events[id]) + 1)
	m.events[id] = append(m.events[id], event)
	return c, nil
}
func (m *memoryRepository) AppendEvent(_ context.Context, event domain.RecoveryEvent) (domain.RecoveryEvent, error) {
	event.Sequence = int64(len(m.events[event.CaseID]) + 1)
	m.events[event.CaseID] = append(m.events[event.CaseID], event)
	return event, nil
}
func (m *memoryRepository) ListEvents(_ context.Context, id domain.ID) ([]domain.RecoveryEvent, error) {
	return m.events[id], nil
}

func TestSyntheticCaseCompletesLifecycleAndAuditsEveryMutation(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	fixed := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	c, err := service.CreateCase(context.Background(), CreateCaseInput{
		LeakType: domain.FailedSubscription, MerchantID: "merchant-1", CustomerID: "customer-1",
		AmountAtRiskMinor: 849900, Currency: "inr", SourceReference: "invoice-1",
		RecoveryDeadline: fixed.Add(7 * 24 * time.Hour), Actor: domain.Actor{Type: "SYSTEM", ID: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := []domain.CaseState{domain.StateDiagnosing, domain.StateActionPending, domain.StatePolicyReview,
		domain.StateScheduled, domain.StateExecuting, domain.StateWaitingOutcome, domain.StateRecovered}
	for _, state := range path {
		c, err = service.Transition(context.Background(), c.ID, TransitionInput{ToState: state, ExpectedVersion: c.Version,
			Actor: domain.Actor{Type: "SYSTEM", ID: "test"}, Payload: json.RawMessage(`{"test":true}`)})
		if err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
	if c.CurrentState != domain.StateRecovered {
		t.Fatalf("got %s", c.CurrentState)
	}
	events, _ := service.Events(context.Background(), c.ID)
	if len(events) != 2+len(path) {
		t.Fatalf("got %d events, want %d", len(events), 2+len(path))
	}
	for i, event := range events {
		if event.Sequence != int64(i+1) {
			t.Fatalf("non-contiguous sequence at %d", i)
		}
	}
	if _, err = service.Transition(context.Background(), c.ID, TransitionInput{ToState: domain.StateActionPending}); err == nil {
		t.Fatal("terminal case accepted transition")
	}
	if len(repo.events[c.ID]) != len(events) {
		t.Fatal("failed transition emitted an event")
	}
}

func TestTransitionRejectsStaleVersion(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo)
	now := time.Now()
	service.now = func() time.Time { return now }
	c, err := service.CreateCase(context.Background(), CreateCaseInput{LeakType: domain.CheckoutAbandonment, MerchantID: "m", CustomerID: "c",
		AmountAtRiskMinor: 100, Currency: "INR", SourceReference: "checkout", RecoveryDeadline: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Transition(context.Background(), c.ID, TransitionInput{ToState: domain.StateDiagnosing, ExpectedVersion: 99})
	if err != ErrConflict {
		t.Fatalf("got %v", err)
	}
}
