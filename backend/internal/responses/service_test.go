package responses

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeStore struct{ seen map[string]bool }

func (f *fakeStore) SaveCustomerResponse(_ context.Context, r Response) (bool, error) {
	if f.seen[r.CorrelationID] {
		return false, nil
	}
	f.seen[r.CorrelationID] = true
	return true, nil
}

func TestIngestIsIdempotentByCorrelationID(t *testing.T) {
	service := NewService(&fakeStore{seen: map[string]bool{}})
	input := Response{CaseID: "case-1", Type: IntentToPay, Payload: json.RawMessage(`{}`), Source: "email", CorrelationID: "reply-1"}
	_, first, err := service.Ingest(context.Background(), input)
	if err != nil || !first {
		t.Fatalf("first ingest failed: %v", err)
	}
	_, second, err := service.Ingest(context.Background(), input)
	if err != nil || second {
		t.Fatalf("duplicate should be idempotent: %v", err)
	}
}

func TestIngestRejectsUnknownType(t *testing.T) {
	service := NewService(&fakeStore{seen: map[string]bool{}})
	if _, _, err := service.Ingest(context.Background(), Response{CaseID: "c", Type: "MAGIC", Payload: json.RawMessage(`{}`), Source: "email", CorrelationID: "x"}); err == nil {
		t.Fatal("expected validation error")
	}
}
