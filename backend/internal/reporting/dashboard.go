package reporting

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type Operational struct {
	Mode                  string            `json:"mode"`
	RevenueAtRiskMinor    int64             `json:"revenue_at_risk_minor"`
	RecoveredMinor        int64             `json:"recovered_minor"`
	NaturalRecoveredMinor int64             `json:"natural_recovered_minor"`
	AgentAttributedMinor  int64             `json:"agent_attributed_minor"`
	ActiveCases           int64             `json:"active_cases"`
	CasesAwaitingReview   int64             `json:"cases_awaiting_review"`
	ActionsScheduled      int64             `json:"actions_scheduled"`
	RecoveryRate          float64           `json:"recovery_rate"`
	Cases                 []map[string]any  `json:"cases"`
	RootCauses            []RootCause       `json:"root_causes"`
	ActionSelections      []ActionSelection `json:"action_selections"`
	RecoveryTimeline      []RecoveryPoint   `json:"recovery_timeline"`
}
type RootCause struct {
	Cause             string `json:"cause"`
	Cases             int64  `json:"cases"`
	AmountAtRiskMinor int64  `json:"amount_at_risk_minor"`
}
type ActionSelection struct {
	Action string `json:"action"`
	Cases  int64  `json:"cases"`
}
type RecoveryPoint struct {
	Day                      string `json:"day"`
	RecoveredMinor           int64  `json:"recovered_minor"`
	CumulativeRecoveredMinor int64  `json:"cumulative_recovered_minor"`
}
type Store interface {
	DashboardOperational(context.Context) (Operational, error)
}
type Service struct {
	store       Store
	resultsRoot string
}

func NewService(store Store, root string) *Service { return &Service{store: store, resultsRoot: root} }
func readObject(path string) map[string]any {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"available": false, "error": err.Error()}
	}
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return map[string]any{"available": false, "error": "invalid evaluation artifact"}
	}
	value["available"] = true
	return value
}
func (s *Service) Get(ctx context.Context) (map[string]any, error) {
	operational, err := s.store.DashboardOperational(ctx)
	if err != nil {
		return nil, err
	}
	synthetic := readObject(filepath.Join(s.resultsRoot, "phase24", "summary.json"))
	budget := readObject(filepath.Join(s.resultsRoot, "phase24", "budget_comparison.json"))
	ablations := readObject(filepath.Join(s.resultsRoot, "phase25", "ablation_summary.json"))
	reconciliation := map[string]any{"checked": false, "message": "Phase 24 v2 artifact is required"}
	if counts, ok := synthetic["case_counts"].(map[string]any); ok {
		generated, go1 := counts["generated"].(float64)
		heldout, go2 := counts["heldout_evaluated_per_strategy"].(float64)
		reconciliation = map[string]any{"checked": go1 && go2, "passed": go1 && go2 && generated >= heldout, "generated_cases": generated, "heldout_cases_per_strategy": heldout}
	}
	return map[string]any{"generated_at_source": "live query plus immutable evaluation artifacts", "operational": operational, "synthetic_evaluation": synthetic, "budget_comparison": budget, "ablations": ablations, "reconciliation": reconciliation}, nil
}
