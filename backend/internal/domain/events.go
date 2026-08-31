package domain

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventRevenueRiskDetected EventType = "REVENUE_RISK_DETECTED"
	EventCaseCreated         EventType = "CASE_CREATED"
	EventDiagnosisStarted    EventType = "DIAGNOSIS_STARTED"
	EventDiagnosisCompleted  EventType = "DIAGNOSIS_COMPLETED"
	EventCandidatesGenerated EventType = "CANDIDATES_GENERATED"
	EventActionPredicted     EventType = "ACTION_PREDICTED"
	EventActionSelected      EventType = "ACTION_SELECTED"
	EventPolicyApproved      EventType = "POLICY_APPROVED"
	EventPolicyDenied        EventType = "POLICY_DENIED"
	EventPolicyEscalated     EventType = "POLICY_ESCALATED"
	EventActionScheduled     EventType = "ACTION_SCHEDULED"
	EventActionExecuted      EventType = "ACTION_EXECUTED"
	EventActionFailed        EventType = "ACTION_FAILED"
	EventOutcomeObserved     EventType = "OUTCOME_OBSERVED"
	EventPromiseCreated      EventType = "PROMISE_CREATED"
	EventPromiseFulfilled    EventType = "PROMISE_FULFILLED"
	EventPromiseBroken       EventType = "PROMISE_BROKEN"
	EventRecoveryCompleted   EventType = "RECOVERY_COMPLETED"
	EventCaseEscalated       EventType = "CASE_ESCALATED"
	EventCaseStopped         EventType = "CASE_STOPPED"
	EventCaseExhausted       EventType = "CASE_EXHAUSTED"
	EventStateTransitioned   EventType = "STATE_TRANSITIONED"
)

var validEventTypes = map[EventType]struct{}{
	EventRevenueRiskDetected: {}, EventCaseCreated: {}, EventDiagnosisStarted: {},
	EventDiagnosisCompleted: {}, EventCandidatesGenerated: {}, EventActionPredicted: {},
	EventActionSelected: {}, EventPolicyApproved: {}, EventPolicyDenied: {},
	EventPolicyEscalated: {}, EventActionScheduled: {}, EventActionExecuted: {},
	EventActionFailed: {}, EventOutcomeObserved: {}, EventPromiseCreated: {},
	EventPromiseFulfilled: {}, EventPromiseBroken: {}, EventRecoveryCompleted: {},
	EventCaseEscalated: {}, EventCaseStopped: {}, EventCaseExhausted: {},
	EventStateTransitioned: {},
}

func (t EventType) IsValid() bool {
	_, ok := validEventTypes[t]
	return ok
}

type Actor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type RecoveryEvent struct {
	ID            ID              `json:"event_id"`
	CaseID        ID              `json:"case_id"`
	Sequence      int64           `json:"sequence"`
	Type          EventType       `json:"event_type"`
	Timestamp     time.Time       `json:"timestamp"`
	Actor         Actor           `json:"actor"`
	Payload       json.RawMessage `json:"payload"`
	ModelVersion  string          `json:"model_version,omitempty"`
	CorrelationID string          `json:"correlation_id"`
}

func EventForTransition(to CaseState) EventType {
	switch to {
	case StateDiagnosing:
		return EventDiagnosisStarted
	case StateScheduled:
		return EventActionScheduled
	case StateExecuting:
		return EventActionExecuted
	case StateWaitingOutcome:
		return EventOutcomeObserved
	case StateRecovered:
		return EventRecoveryCompleted
	case StateEscalated:
		return EventCaseEscalated
	case StateStopped:
		return EventCaseStopped
	case StateExhausted:
		return EventCaseExhausted
	default:
		return EventStateTransitioned
	}
}
