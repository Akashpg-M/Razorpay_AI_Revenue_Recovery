package replay

import (
	"context"
	"encoding/json"
	"revenue-recovery/backend/internal/domain"
)

type View struct {
	Case               domain.RecoveryCase `json:"case"`
	Merchant           json.RawMessage     `json:"merchant"`
	Events             []json.RawMessage   `json:"events"`
	Decisions          []json.RawMessage   `json:"decisions"`
	Candidates         []json.RawMessage   `json:"candidates"`
	EconomicGates      []json.RawMessage   `json:"economic_gates"`
	PolicyEvaluations  []json.RawMessage   `json:"policy_evaluations"`
	Actions            []json.RawMessage   `json:"actions"`
	Schedules          []json.RawMessage   `json:"schedules"`
	Executions         []json.RawMessage   `json:"executions"`
	Promises           []json.RawMessage   `json:"promises_to_pay"`
	PromiseEvents      []json.RawMessage   `json:"promise_events"`
	PromiseChecks      []json.RawMessage   `json:"promise_checks"`
	HumanReviews       []json.RawMessage   `json:"human_reviews"`
	Attributions       []json.RawMessage   `json:"attributions"`
	ProviderReferences []json.RawMessage   `json:"provider_references"`
	WebhookEvents      []json.RawMessage   `json:"webhook_events"`
	FeedbackRecords    []json.RawMessage   `json:"feedback_records"`
	Provenance         map[string]any      `json:"provenance"`
}
type Store interface {
	GetReplay(context.Context, domain.ID) (View, error)
}
