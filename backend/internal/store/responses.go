package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/recovery"
	"revenue-recovery/backend/internal/responses"
)

func (p *Postgres) SaveCustomerResponse(ctx context.Context, response responses.Response) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var customerID domain.ID
	var state domain.CaseState
	var version int64
	if err = tx.QueryRow(ctx, `SELECT customer_id,current_state,version FROM recovery_cases WHERE id=$1 FOR UPDATE`, response.CaseID).Scan(&customerID, &state, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, recovery.ErrNotFound
		}
		return false, err
	}
	command, err := tx.Exec(ctx, `INSERT INTO customer_responses(id,case_id,response_type,payload,source,received_at,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(correlation_id) DO NOTHING`, response.ID, response.CaseID, response.Type, response.Payload, response.Source, response.ReceivedAt, response.CorrelationID)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}

	if response.Type == responses.OptOut {
		if _, err = tx.Exec(ctx, `UPDATE customers SET opted_out=TRUE,updated_at=$2 WHERE id=$1`, customerID, response.ReceivedAt); err != nil {
			return false, err
		}
	}
	eventTypes := []domain.EventType{domain.EventCustomerResponded}
	// A valid promise is a hold signal. Promise creation and its durable due check
	// are owned by the promise service after this transaction commits.
	if state == domain.StateWaitingOutcome && response.Type != responses.Acknowledgement && response.Type != responses.PromiseToPay {
		if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='REASSESSING',version=version+1,updated_at=$2 WHERE id=$1`, response.CaseID, response.ReceivedAt); err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='ACTION_PENDING',version=version+1,updated_at=$2 WHERE id=$1`, response.CaseID, response.ReceivedAt); err != nil {
			return false, err
		}
		eventTypes = append(eventTypes, domain.EventStateTransitioned, domain.EventStateTransitioned)
	}
	sequence, err := nextSequence(ctx, tx, response.CaseID)
	if err != nil {
		return false, err
	}
	payload, _ := json.Marshal(map[string]any{"response_id": response.ID, "response_type": response.Type, "source": response.Source})
	for index, eventType := range eventTypes {
		eventPayload := payload
		if eventType == domain.EventStateTransitioned {
			to := domain.StateReassessing
			if index == len(eventTypes)-1 {
				to = domain.StateActionPending
			}
			eventPayload, _ = json.Marshal(map[string]any{"response_id": response.ID, "to_state": to})
		}
		event := domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: response.CaseID, Sequence: sequence + int64(index), Type: eventType, Timestamp: response.ReceivedAt, Actor: domain.Actor{Type: "CUSTOMER", ID: string(customerID)}, Payload: eventPayload, CorrelationID: response.CorrelationID}
		if err = insertEvent(ctx, tx, event); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
