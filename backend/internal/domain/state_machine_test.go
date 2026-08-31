package domain

import "testing"

func TestCompleteRecoveryLifecycle(t *testing.T) {
	path := []CaseState{
		StateDetected, StateDiagnosing, StateActionPending, StatePolicyReview,
		StateScheduled, StateExecuting, StateWaitingOutcome, StateRecovered,
	}
	for i := 0; i < len(path)-1; i++ {
		if err := ValidateTransition(path[i], path[i+1]); err != nil {
			t.Fatalf("expected %s -> %s to be valid: %v", path[i], path[i+1], err)
		}
	}
}

func TestReassessmentLifecycle(t *testing.T) {
	path := []CaseState{StateWaitingOutcome, StateReassessing, StateActionPending}
	for i := 0; i < len(path)-1; i++ {
		if err := ValidateTransition(path[i], path[i+1]); err != nil {
			t.Fatalf("expected %s -> %s to be valid: %v", path[i], path[i+1], err)
		}
	}
}

func TestTerminalStatesRejectTransitions(t *testing.T) {
	for _, terminal := range []CaseState{StateRecovered, StateExhausted, StateStopped} {
		if err := ValidateTransition(terminal, StateActionPending); err == nil {
			t.Fatalf("expected %s -> ACTION_PENDING to fail", terminal)
		}
	}
}

func TestUnknownStatesRejected(t *testing.T) {
	if err := ValidateTransition(CaseState("UNKNOWN"), StateDetected); err == nil {
		t.Fatal("expected unknown source to fail")
	}
	if err := ValidateTransition(StateDetected, CaseState("UNKNOWN")); err == nil {
		t.Fatal("expected unknown target to fail")
	}
}
