package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/decisioning"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/orchestrator"
	"revenue-recovery/backend/internal/recovery"
)

// PrepareForDecision advances only the lifecycle states that are prerequisites
// for decisioning. It never weakens the scheduler's ACTION_PENDING/version
// check, and a repeated request after scheduling is rejected before another
// decision can be persisted.
func (p *Postgres) PrepareForDecision(ctx context.Context, caseID domain.ID, now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var state domain.CaseState
	var version int64
	if err = tx.QueryRow(ctx, `SELECT current_state,version FROM recovery_cases WHERE id=$1 FOR UPDATE`, caseID).Scan(&state, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return recovery.ErrNotFound
		}
		return err
	}

	targets := []struct {
		from      domain.CaseState
		to        domain.CaseState
		eventType domain.EventType
	}{}
	switch state {
	case domain.StateDetected:
		targets = append(targets,
			struct {
				from      domain.CaseState
				to        domain.CaseState
				eventType domain.EventType
			}{domain.StateDetected, domain.StateDiagnosing, domain.EventDiagnosisStarted},
			struct {
				from      domain.CaseState
				to        domain.CaseState
				eventType domain.EventType
			}{domain.StateDiagnosing, domain.StateActionPending, domain.EventDiagnosisCompleted},
		)
	case domain.StateDiagnosing:
		targets = append(targets, struct {
			from      domain.CaseState
			to        domain.CaseState
			eventType domain.EventType
		}{domain.StateDiagnosing, domain.StateActionPending, domain.EventDiagnosisCompleted})
	case domain.StateReassessing:
		targets = append(targets, struct {
			from      domain.CaseState
			to        domain.CaseState
			eventType domain.EventType
		}{domain.StateReassessing, domain.StateActionPending, domain.EventStateTransitioned})
	case domain.StateActionPending:
		return tx.Commit(ctx)
	default:
		return fmt.Errorf("%w: case in %s cannot start a decision", recovery.ErrConflict, state)
	}

	sequence, err := nextSequence(ctx, tx, caseID)
	if err != nil {
		return err
	}
	for index, transition := range targets {
		if !domain.CanTransition(transition.from, transition.to) {
			return fmt.Errorf("invalid configured decision transition: %s -> %s", transition.from, transition.to)
		}
		command, updateErr := tx.Exec(ctx, `UPDATE recovery_cases SET current_state=$2,version=version+1,updated_at=$3 WHERE id=$1 AND current_state=$4 AND version=$5`, caseID, transition.to, now, transition.from, version)
		if updateErr != nil {
			return updateErr
		}
		if command.RowsAffected() != 1 {
			return recovery.ErrConflict
		}
		payload, _ := json.Marshal(map[string]any{"from_state": transition.from, "to_state": transition.to, "previous_version": version})
		event := domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: caseID, Sequence: sequence + int64(index), Type: transition.eventType, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "decision-lifecycle-v1"}, Payload: payload, CorrelationID: fmt.Sprintf("decision-preparation:%s:%d", caseID, version)}
		if err = insertEvent(ctx, tx, event); err != nil {
			return err
		}
		version++
	}
	return tx.Commit(ctx)
}

// SaveDecisionAndSchedule prevents the old partial-commit failure where the
// decision/policy rows committed and scheduling then failed in a second
// transaction. Approved work and its schedule now commit or roll back together.
func (p *Postgres) SaveDecisionAndSchedule(ctx context.Context, snapshot decisioning.Snapshot) (*orchestrator.ScheduledAction, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = saveDecisionTx(ctx, tx, snapshot); err != nil {
		return nil, err
	}

	var scheduled *orchestrator.ScheduledAction
	if snapshot.Policy.Decision == "APPROVE" {
		if snapshot.Gate.Decision != "ALLOW" {
			return nil, errors.New("economically blocked decision may not be scheduled")
		}
		if snapshot.Decision.Optimization.Selected.Action != domain.ActionWait {
			scheduled, err = scheduleDecisionTx(ctx, tx, snapshot)
			if err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return scheduled, nil
}
