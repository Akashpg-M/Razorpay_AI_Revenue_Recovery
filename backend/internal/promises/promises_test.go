package promises

import (
	"context"
	"errors"
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

func TestDeterministicExtraction(t *testing.T) {
	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.FixedZone("IST", 19800))
	for _, text := range []string{"I will pay tomorrow at 10am", "I'll pay friday", "pay on 2026-09-05 14:30"} {
		result, err := Extract(text, nil, now, now.Location())
		if err != nil || !result.PromisedFor.After(now) || result.ExtractorVersion != ExtractorVersion {
			t.Fatalf("%q: %+v %v", text, result, err)
		}
	}
}

type fakeStore struct {
	promise       domain.PromiseToPay
	created       bool
	resolveStatus string
}

func (f *fakeStore) CreatePromise(_ context.Context, p domain.PromiseToPay, _ string) (domain.PromiseToPay, bool, error) {
	if f.created {
		return f.promise, false, nil
	}
	f.created = true
	f.promise = p
	return p, true, nil
}
func (f *fakeStore) GetPromise(context.Context, domain.ID) (domain.PromiseToPay, error) {
	return f.promise, nil
}
func (f *fakeStore) ListPromises(context.Context, domain.ID) ([]domain.PromiseToPay, error) {
	return []domain.PromiseToPay{f.promise}, nil
}
func (f *fakeStore) CancelPromise(_ context.Context, _ domain.ID, _ string, now time.Time) (domain.PromiseToPay, error) {
	f.promise.Status = "CANCELLED"
	f.promise.ResolvedAt = &now
	return f.promise, nil
}
func (f *fakeStore) ClaimDuePromise(context.Context, string, time.Time, time.Duration) (domain.PromiseToPay, error) {
	if !f.created {
		return domain.PromiseToPay{}, ErrNoDuePromise
	}
	return f.promise, nil
}
func (f *fakeStore) ResolveDuePromise(_ context.Context, _ domain.ID, _ time.Time) (domain.PromiseToPay, error) {
	if f.resolveStatus == "" {
		return domain.PromiseToPay{}, errors.New("missing status")
	}
	f.promise.Status = f.resolveStatus
	return f.promise, nil
}

type fakeReassessor struct{ caseID domain.ID }

func (f *fakeReassessor) Reassess(_ context.Context, id domain.ID) error { f.caseID = id; return nil }

func TestPromiseCreationIsIdempotentAndBrokenPromiseReassesses(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{resolveStatus: "BROKEN"}
	reassessor := &fakeReassessor{}
	service := NewService(store, reassessor)
	service.now = func() time.Time { return now }
	due := now.Add(time.Hour)
	first, created, err := service.Create(context.Background(), CreateInput{CaseID: "case-1", PromisedFor: &due, CorrelationID: "same"})
	if err != nil || !created {
		t.Fatalf("first create: %v %v", created, err)
	}
	second, created, err := service.Create(context.Background(), CreateInput{CaseID: "case-1", PromisedFor: &due, CorrelationID: "same"})
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("duplicate create was not idempotent")
	}
	if err = service.RunDueCheck(context.Background(), "worker"); err != nil || reassessor.caseID != "case-1" {
		t.Fatalf("broken lifecycle failed: %v", err)
	}
}
func TestAmbiguousAndPastPromiseRejected(t *testing.T) {
	now := time.Now()
	if _, err := Extract("soon", nil, now, time.UTC); err == nil {
		t.Fatal("expected ambiguity")
	}
	past := now.Add(-time.Hour)
	if _, err := Extract("", &past, now, time.UTC); err == nil {
		t.Fatal("expected past rejection")
	}
}
