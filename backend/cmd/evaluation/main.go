package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"revenue-recovery/backend/internal/resilience"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func writeJSON(path string, value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	must(err)
	must(os.WriteFile(path, append(encoded, '\n'), 0o644))
}

func main() {
	root := "../decision-service/evaluation/results"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	safetyDir := filepath.Join(root, "phase26")
	faultDir := filepath.Join(root, "phase27")
	must(os.MkdirAll(safetyDir, 0o755))
	must(os.MkdirAll(faultDir, 0o755))
	safety := resilience.RunAuthorizationSuite()
	restricted, blocked, unsafe := 0, 0, 0
	for _, item := range safety {
		if item.Scenario != "valid_control" {
			restricted++
			if item.ExternalCallCount == 0 {
				blocked++
			}
			unsafe += item.ExternalCallCount
		}
	}
	contract := map[string]any{"version": "safety-evaluation-v1", "unsafe_action_definition": "external execution while current authoritative policy requires DENY, ESCALATE, or STOP", "boundary": "orchestrator.Worker.RunOnce", "target": "zero unsafe external effects in covered scenarios"}
	summary := map[string]any{"evaluation_version": "phase26-safety-v1", "restricted_scenarios": restricted, "correctly_blocked_or_escalated_or_stopped": blocked, "unsafe_attempts": unsafe, "unsafe_external_side_effects": unsafe, "safety_pass_rate": float64(blocked) / float64(restricted), "all_passed": blocked == restricted}
	writeJSON(filepath.Join(safetyDir, "safety_summary.json"), summary)
	writeJSON(filepath.Join(safetyDir, "invariant_results.json"), map[string]any{"contract": contract, "results": safety})
	f, err := os.Create(filepath.Join(safetyDir, "safety_cases.csv"))
	must(err)
	w := csv.NewWriter(f)
	must(w.Write([]string{"scenario", "executor_invoked", "external_effect_count", "expected_result", "pass", "suppression_reason"}))
	for _, item := range safety {
		must(w.Write([]string{item.Scenario, strconv.FormatBool(item.ExternalCallCount > 0), strconv.Itoa(item.ProviderEffectCount), map[bool]string{true: "APPROVE", false: "DENY_ESCALATE_OR_STOP"}[item.Scenario == "valid_control"], strconv.FormatBool(item.Passed), item.SuppressionReason}))
	}
	w.Flush()
	must(w.Error())
	must(f.Close())
	faults := resilience.RunFaultSuite()
	duplicates := 0
	for _, item := range faults {
		if item.ProviderEffectCount > 1 {
			duplicates += item.ProviderEffectCount - 1
		}
	}
	faultSummary := map[string]any{"evaluation_version": "phase27-reliability-v1", "fault_scenarios": len(faults), "passed": countPassed(faults), "duplicate_external_effects": duplicates, "delivery_semantics": "at-least-once processing with idempotent or reconciled side effects", "exactly_once_claimed": false}
	writeJSON(filepath.Join(faultDir, "reliability_summary.json"), faultSummary)
	jsonl, err := os.Create(filepath.Join(faultDir, "fault_runs.jsonl"))
	must(err)
	encoder := json.NewEncoder(jsonl)
	for _, item := range faults {
		must(encoder.Encode(item))
	}
	must(jsonl.Close())
	m, err := os.Create(filepath.Join(faultDir, "reliability_matrix.csv"))
	must(err)
	mw := csv.NewWriter(m)
	must(mw.Write([]string{"fault", "events_delivered", "worker_attempts", "claims", "reclaims", "provider_calls", "external_effects", "duplicate_effects", "duplicates_blocked", "reconciliations", "final_action_state", "final_case_state", "pass"}))
	for _, item := range faults {
		duplicate := item.ProviderEffectCount - 1
		if duplicate < 0 {
			duplicate = 0
		}
		must(mw.Write([]string{item.Scenario, strconv.Itoa(item.EventsDelivered), strconv.Itoa(item.ExecutionAttemptCount), strconv.Itoa(item.Claims), strconv.Itoa(item.Reclaims), strconv.Itoa(item.ExternalCallCount), strconv.Itoa(item.ProviderEffectCount), strconv.Itoa(duplicate), strconv.Itoa(item.DuplicatesBlocked), strconv.Itoa(item.ReconciliationEvents), item.FinalActionState, item.FinalCaseState, strconv.FormatBool(item.Passed)}))
	}
	mw.Flush()
	must(mw.Error())
	must(m.Close())
	fmt.Printf("wrote Phase 26 and 27 evaluation artifacts under %s\n", root)
}
func countPassed(results []resilience.Result) int {
	value := 0
	for _, item := range results {
		if item.Passed {
			value++
		}
	}
	return value
}
