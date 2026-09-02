package observability

import (
	"context"
	"testing"
)

type fakeStore struct{ value Snapshot }

func (f fakeStore) OperationalSnapshot(context.Context) (Snapshot, error) { return f.value, nil }
func TestSnapshotDerivesActionableAlerts(t *testing.T) {
	value, err := New(fakeStore{Snapshot{Queue: Queue{MaxLagSeconds: 301}, Execution: Execution{Failed: 1}, Recovery: Recovery{ExpiredPromises: 1}}}).Snapshot(context.Background())
	if err != nil || len(value.Alerts) != 3 {
		t.Fatalf("got %d alerts, err=%v", len(value.Alerts), err)
	}
}

func TestSnapshotReturnsEmptyAlertCollectionInsteadOfNil(t *testing.T) {
	value, err := New(fakeStore{Snapshot{}}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if value.Alerts == nil || len(value.Alerts) != 0 {
		t.Fatalf("alerts must be an empty non-nil collection: %#v", value.Alerts)
	}
}
