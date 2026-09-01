package store

import (
	"context"
	"revenue-recovery/backend/internal/observability"
)

func (p *Postgres) OperationalSnapshot(ctx context.Context) (observability.Snapshot, error) {
	var v observability.Snapshot
	err := p.pool.QueryRow(ctx, `SELECT COALESCE((SELECT value FROM platform_metadata WHERE key='schema_version'),''), (SELECT COUNT(*) FROM scheduled_actions WHERE status='PENDING'),(SELECT COUNT(*) FROM scheduled_actions WHERE status IN('CLAIMED','EXECUTING','OBSERVATION_CLAIMED')),(SELECT COUNT(*) FROM scheduled_actions WHERE status='FAILED'),COALESCE((SELECT EXTRACT(EPOCH FROM NOW()-MIN(scheduled_for)) FROM scheduled_actions WHERE status='PENDING' AND scheduled_for<NOW()),0),(SELECT COUNT(*) FROM executions WHERE status='SUCCEEDED'),(SELECT COUNT(*) FROM executions WHERE status IN('FAILED','REJECTED_BY_PROVIDER')),(SELECT COUNT(*) FROM executions WHERE status='TIMED_OUT'),(SELECT COUNT(*) FROM scheduled_actions WHERE status='RETRY_PENDING'),(SELECT COUNT(*) FROM recovery_cases WHERE current_state NOT IN('RECOVERED','ESCALATED','EXHAUSTED','STOPPED')),(SELECT COUNT(*) FROM recovery_cases WHERE current_state='RECOVERED'),(SELECT COUNT(*) FROM recovery_cases WHERE current_state IN('ESCALATED','EXHAUSTED')),(SELECT COUNT(*) FROM recovery_cases WHERE current_state='STOPPED'),(SELECT COUNT(*) FROM promises_to_pay WHERE status='ACTIVE' AND due_at<NOW())`).Scan(&v.SchemaVersion, &v.Queue.Pending, &v.Queue.Running, &v.Queue.Failed, &v.Queue.MaxLagSeconds, &v.Execution.Succeeded, &v.Execution.Failed, &v.Execution.TimedOut, &v.Execution.Retrying, &v.Recovery.Active, &v.Recovery.Recovered, &v.Recovery.Escalated, &v.Recovery.Stopped, &v.Recovery.ExpiredPromises)
	return v, err
}
