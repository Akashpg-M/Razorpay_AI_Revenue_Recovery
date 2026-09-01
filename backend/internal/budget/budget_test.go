package budget

import (
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/portfolio"
	"testing"
	"time"
)

func TestGreedyRespectsConstraints(t *testing.T) {
	now := time.Now()
	items := []portfolio.Item{{CaseID: "a", DecisionID: "d1", Action: domain.ActionSendReminder, ExpectedNERVMinor: 1000, Rank: 1, CreatedAt: now}, {CaseID: "b", DecisionID: "d2", Action: domain.ActionSendReminder, ExpectedNERVMinor: 900, Rank: 2, CreatedAt: now}}
	run := Allocate(items, Limits{SpendMinor: 25, Contacts: 1, Retries: 2, DiscountMinor: 0, HumanReviews: 0}, GreedyVersion, now, "m", "p")
	if !run.Allocations[0].Included || run.Allocations[1].Included || run.Totals.SpendMinor > 25 {
		t.Fatalf("constraints not enforced: %#v", run)
	}
}
