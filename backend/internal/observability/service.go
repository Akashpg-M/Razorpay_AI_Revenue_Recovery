package observability

import (
	"context"
	"time"
)

type Snapshot struct {
	GeneratedAt   time.Time `json:"generated_at"`
	SchemaVersion string    `json:"schema_version"`
	Queue         Queue     `json:"queue"`
	Execution     Execution `json:"execution"`
	Recovery      Recovery  `json:"recovery"`
	Webhooks      Webhooks  `json:"webhooks"`
	Alerts        []Alert   `json:"alerts"`
}
type Webhooks struct {
	Received     int64      `json:"received"`
	Processed    int64      `json:"processed"`
	Failed       int64      `json:"failed"`
	LastReceived *time.Time `json:"last_received_at,omitempty"`
}
type Queue struct {
	Pending       int64   `json:"pending"`
	Running       int64   `json:"running"`
	Failed        int64   `json:"failed"`
	MaxLagSeconds float64 `json:"max_lag_seconds"`
}
type Execution struct {
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	TimedOut  int64 `json:"timed_out"`
	Retrying  int64 `json:"retrying"`
}
type Recovery struct {
	Active          int64 `json:"active_cases"`
	Recovered       int64 `json:"recovered_cases"`
	Escalated       int64 `json:"escalated_cases"`
	Stopped         int64 `json:"stopped_cases"`
	ExpiredPromises int64 `json:"expired_promises"`
}
type Alert struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
type Store interface {
	OperationalSnapshot(context.Context) (Snapshot, error)
}
type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	v, err := s.store.OperationalSnapshot(ctx)
	if err != nil {
		return v, err
	}
	// Keep the JSON contract stable: an empty collection must be [] rather
	// than null so API consumers can safely iterate it.
	if v.Alerts == nil {
		v.Alerts = []Alert{}
	}
	v.GeneratedAt = s.now().UTC()
	if v.Queue.MaxLagSeconds > 300 {
		v.Alerts = append(v.Alerts, Alert{"QUEUE_LAG_HIGH", "warning", "Oldest due action is more than five minutes late"})
	}
	if v.Queue.Failed > 0 || v.Execution.Failed > 0 {
		v.Alerts = append(v.Alerts, Alert{"EXECUTION_FAILURES", "critical", "Failed scheduled actions or executions require attention"})
	}
	if v.Recovery.ExpiredPromises > 0 {
		v.Alerts = append(v.Alerts, Alert{"PROMISE_CHECKS_OVERDUE", "warning", "Active promises are past their due time"})
	}
	return v, nil
}
