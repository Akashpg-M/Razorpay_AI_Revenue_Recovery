package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/recovery"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (p *Postgres) CreateCase(ctx context.Context, c domain.RecoveryCase, events []domain.RecoveryEvent) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO recovery_cases
		(id, leak_type, merchant_id, customer_id, amount_at_risk_minor, currency, source_reference,
		source_status, failure_or_leak_context, customer_context_snapshot, merchant_policy_snapshot,
		current_state, recovery_deadline, recovered_amount_minor, attribution_status, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		c.ID, c.LeakType, c.MerchantID, c.CustomerID, c.AmountAtRiskMinor, c.Currency, c.SourceReference,
		c.SourceStatus, c.FailureOrLeakContext, c.CustomerContextSnapshot, c.MerchantPolicySnapshot,
		c.CurrentState, c.RecoveryDeadline, c.RecoveredAmountMinor, c.AttributionStatus, c.Version, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	for i := range events {
		events[i].Sequence = int64(i + 1)
		if err := insertEvent(ctx, tx, events[i]); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) GetCase(ctx context.Context, id domain.ID) (domain.RecoveryCase, error) {
	row := p.pool.QueryRow(ctx, caseSelect+` WHERE id=$1`, id)
	c, err := scanCase(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecoveryCase{}, recovery.ErrNotFound
	}
	return c, err
}

func (p *Postgres) GetCaseBySource(ctx context.Context, merchantID domain.ID, sourceReference string) (domain.RecoveryCase, error) {
	row := p.pool.QueryRow(ctx, caseSelect+` WHERE merchant_id=$1 AND source_reference=$2`, merchantID, sourceReference)
	c, err := scanCase(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecoveryCase{}, recovery.ErrNotFound
	}
	return c, err
}

func (p *Postgres) TransitionCase(ctx context.Context, caseID domain.ID, expectedVersion int64, to domain.CaseState, event domain.RecoveryEvent) (domain.RecoveryCase, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.RecoveryCase{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `UPDATE recovery_cases SET current_state=$1, version=version+1, updated_at=$2
		WHERE id=$3 AND version=$4 RETURNING id,leak_type,merchant_id,customer_id,amount_at_risk_minor,currency,
		source_reference,source_status,failure_or_leak_context,customer_context_snapshot,merchant_policy_snapshot,
		current_state,recovery_deadline,recovered_amount_minor,attribution_status,version,created_at,updated_at`,
		to, event.Timestamp, caseID, expectedVersion)
	c, err := scanCase(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecoveryCase{}, recovery.ErrConflict
	}
	if err != nil {
		return domain.RecoveryCase{}, err
	}
	event.Sequence, err = nextSequence(ctx, tx, caseID)
	if err != nil {
		return domain.RecoveryCase{}, err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return domain.RecoveryCase{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.RecoveryCase{}, err
	}
	return c, nil
}

func (p *Postgres) AppendEvent(ctx context.Context, event domain.RecoveryEvent) (domain.RecoveryEvent, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.RecoveryEvent{}, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err = tx.QueryRow(ctx, `SELECT id FROM recovery_cases WHERE id=$1 FOR UPDATE`, event.CaseID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RecoveryEvent{}, recovery.ErrNotFound
		}
		return domain.RecoveryEvent{}, err
	}
	event.Sequence, err = nextSequence(ctx, tx, event.CaseID)
	if err != nil {
		return domain.RecoveryEvent{}, err
	}
	if err = insertEvent(ctx, tx, event); err != nil {
		return domain.RecoveryEvent{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.RecoveryEvent{}, err
	}
	return event, nil
}

func (p *Postgres) ListEvents(ctx context.Context, caseID domain.ID) ([]domain.RecoveryEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,case_id,sequence,event_type,occurred_at,actor,payload,
		COALESCE(model_version,''),correlation_id FROM recovery_events WHERE case_id=$1 ORDER BY sequence`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.RecoveryEvent, 0)
	for rows.Next() {
		var event domain.RecoveryEvent
		var actorJSON []byte
		if err := rows.Scan(&event.ID, &event.CaseID, &event.Sequence, &event.Type, &event.Timestamp,
			&actorJSON, &event.Payload, &event.ModelVersion, &event.CorrelationID); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(actorJSON, &event.Actor); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

const caseSelect = `SELECT id,leak_type,merchant_id,customer_id,amount_at_risk_minor,currency,
	source_reference,source_status,failure_or_leak_context,customer_context_snapshot,merchant_policy_snapshot,
	current_state,recovery_deadline,recovered_amount_minor,attribution_status,version,created_at,updated_at FROM recovery_cases`

type rowScanner interface{ Scan(...any) error }

func scanCase(row rowScanner) (domain.RecoveryCase, error) {
	var c domain.RecoveryCase
	err := row.Scan(&c.ID, &c.LeakType, &c.MerchantID, &c.CustomerID, &c.AmountAtRiskMinor, &c.Currency,
		&c.SourceReference, &c.SourceStatus, &c.FailureOrLeakContext, &c.CustomerContextSnapshot,
		&c.MerchantPolicySnapshot, &c.CurrentState, &c.RecoveryDeadline, &c.RecoveredAmountMinor,
		&c.AttributionStatus, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func nextSequence(ctx context.Context, tx pgx.Tx, caseID domain.ID) (int64, error) {
	var sequence int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM recovery_events WHERE case_id=$1`, caseID).Scan(&sequence)
	return sequence, err
}

func insertEvent(ctx context.Context, tx pgx.Tx, event domain.RecoveryEvent) error {
	actor, err := json.Marshal(event.Actor)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO recovery_events
		(id,case_id,sequence,event_type,occurred_at,actor,payload,model_version,correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)`, event.ID, event.CaseID, event.Sequence,
		event.Type, event.Timestamp, actor, event.Payload, event.ModelVersion, event.CorrelationID)
	return err
}
