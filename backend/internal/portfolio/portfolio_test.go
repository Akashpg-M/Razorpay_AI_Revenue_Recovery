package portfolio

import (
	"revenue-recovery/backend/internal/domain"
	"testing"
	"time"
)

func TestRankIsDeterministicAndRewardsUrgency(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	input := []Candidate{{CaseID: "later", ExpectedNERVMinor: 1000, Deadline: now.Add(48 * time.Hour), RecoverabilityBPS: 8000}, {CaseID: "urgent", ExpectedNERVMinor: 1000, Deadline: now.Add(time.Hour), RecoverabilityBPS: 8000}}
	first := Rank(input, now, "run")
	second := Rank(input, now, "run")
	if first[0].CaseID != domain.ID("urgent") || first[0].CaseID != second[0].CaseID {
		t.Fatalf("unexpected deterministic order: %#v", first)
	}
}
