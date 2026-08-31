package domain

import "fmt"

type CaseState string

const (
	StateDetected       CaseState = "DETECTED"
	StateDiagnosing     CaseState = "DIAGNOSING"
	StateActionPending  CaseState = "ACTION_PENDING"
	StatePolicyReview   CaseState = "POLICY_REVIEW"
	StateScheduled      CaseState = "SCHEDULED"
	StateExecuting      CaseState = "EXECUTING"
	StateWaitingOutcome CaseState = "WAITING_OUTCOME"
	StateReassessing    CaseState = "REASSESSING"
	StateRecovered      CaseState = "RECOVERED"
	StateEscalated      CaseState = "ESCALATED"
	StateExhausted      CaseState = "EXHAUSTED"
	StateStopped        CaseState = "STOPPED"
)

var transitions = map[CaseState]map[CaseState]struct{}{
	StateDetected:       set(StateDiagnosing, StateStopped),
	StateDiagnosing:     set(StateActionPending, StateEscalated, StateStopped),
	StateActionPending:  set(StatePolicyReview, StateEscalated, StateExhausted, StateStopped),
	StatePolicyReview:   set(StateScheduled, StateReassessing, StateEscalated, StateStopped),
	StateScheduled:      set(StateExecuting, StateReassessing, StateStopped),
	StateExecuting:      set(StateWaitingOutcome, StateReassessing, StateEscalated, StateStopped),
	StateWaitingOutcome: set(StateRecovered, StateReassessing, StateEscalated, StateExhausted, StateStopped),
	StateReassessing:    set(StateActionPending, StateRecovered, StateEscalated, StateExhausted, StateStopped),
	StateEscalated:      set(StateReassessing, StateActionPending, StateRecovered, StateExhausted, StateStopped),
	StateRecovered:      set(),
	StateExhausted:      set(),
	StateStopped:        set(),
}

func set(states ...CaseState) map[CaseState]struct{} {
	result := make(map[CaseState]struct{}, len(states))
	for _, state := range states {
		result[state] = struct{}{}
	}
	return result
}

func (s CaseState) IsValid() bool {
	_, ok := transitions[s]
	return ok
}

func (s CaseState) IsTerminal() bool {
	return s == StateRecovered || s == StateExhausted || s == StateStopped
}

func CanTransition(from, to CaseState) bool {
	allowed, ok := transitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

func ValidateTransition(from, to CaseState) error {
	if !from.IsValid() {
		return fmt.Errorf("unknown source state %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("unknown target state %q", to)
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid recovery case transition: %s -> %s", from, to)
	}
	return nil
}

func AllowedTransitions(from CaseState) []CaseState {
	allowed := transitions[from]
	result := make([]CaseState, 0, len(allowed))
	for state := range allowed {
		result = append(result, state)
	}
	return result
}
