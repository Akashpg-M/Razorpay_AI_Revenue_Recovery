package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/promises"
	"revenue-recovery/backend/internal/recovery"
)

const promiseColumns = `id,case_id,customer_id,status,due_at,confidence,source,created_at,resolved_at,promised_amount_minor,extractor_version,extraction_timestamp,source_response_id,fulfilled_at,broken_at,expired_at,cancelled_at,COALESCE(verification_reference,'')`

func scanPromise(row rowScanner) (domain.PromiseToPay, error) {
	var p domain.PromiseToPay
	err := row.Scan(&p.ID, &p.CaseID, &p.CustomerID, &p.Status, &p.DueAt, &p.Confidence, &p.Source, &p.CreatedAt, &p.ResolvedAt, &p.PromisedAmountMinor, &p.ExtractorVersion, &p.ExtractionTimestamp, &p.SourceResponseID, &p.FulfilledAt, &p.BrokenAt, &p.ExpiredAt, &p.CancelledAt, &p.VerificationReference)
	return p, err
}

func (p *Postgres) CreatePromise(ctx context.Context, promise domain.PromiseToPay, correlationID string) (domain.PromiseToPay, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return promise, false, err
	}
	defer tx.Rollback(ctx)
	var customerID domain.ID
	if err = tx.QueryRow(ctx, `SELECT customer_id FROM recovery_cases WHERE id=$1 FOR UPDATE`, promise.CaseID).Scan(&customerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return promise, false, recovery.ErrNotFound
		}
		return promise, false, err
	}
	promise.CustomerID = customerID
	if promise.SourceResponseID != nil {
		existing, queryErr := scanPromise(tx.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE source_response_id=$1`, *promise.SourceResponseID))
		if queryErr == nil {
			_ = tx.Commit(ctx)
			return existing, false, nil
		}
		if !errors.Is(queryErr, pgx.ErrNoRows) {
			return promise, false, queryErr
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO promises_to_pay(id,case_id,customer_id,status,due_at,confidence,source,created_at,promised_amount_minor,extractor_version,extraction_timestamp,source_response_id)VALUES($1,$2,$3,'ACTIVE',$4,$5,$6,$7,$8,$9,$10,$11)`, promise.ID, promise.CaseID, promise.CustomerID, promise.DueAt, promise.Confidence, promise.Source, promise.CreatedAt, promise.PromisedAmountMinor, promise.ExtractorVersion, promise.ExtractionTimestamp, promise.SourceResponseID)
	if err != nil {
		return promise, false, err
	}
	checkID := id.New()
	_, err = tx.Exec(ctx, `INSERT INTO promise_checks(id,promise_id,case_id,scheduled_for,status,created_at)VALUES($1,$2,$3,$4,'PENDING',$5)`, checkID, promise.ID, promise.CaseID, promise.DueAt, promise.CreatedAt)
	if err != nil {
		return promise, false, err
	}
	payload, _ := json.Marshal(map[string]any{"promise_id": promise.ID, "promised_for": promise.DueAt, "confidence": promise.Confidence, "extractor_version": promise.ExtractorVersion, "check_id": checkID})
	_, err = tx.Exec(ctx, `INSERT INTO promise_events(id,promise_id,case_id,from_status,to_status,reason_code,payload,occurred_at,correlation_id)VALUES($1,$2,$3,NULL,'ACTIVE','CUSTOMER_PROMISE_CREATED',$4,$5,$6)`, id.New(), promise.ID, promise.CaseID, payload, promise.CreatedAt, correlationID)
	if err != nil {
		return promise, false, err
	}
	sequence, err := nextSequence(ctx, tx, promise.CaseID)
	if err != nil {
		return promise, false, err
	}
	events := []domain.RecoveryEvent{{ID: domain.ID(id.New()), CaseID: promise.CaseID, Sequence: sequence, Type: domain.EventPromiseCreated, Timestamp: promise.CreatedAt, Actor: domain.Actor{Type: "CUSTOMER", ID: string(customerID)}, Payload: payload, CorrelationID: correlationID}, {ID: domain.ID(id.New()), CaseID: promise.CaseID, Sequence: sequence + 1, Type: domain.EventPromiseCheckScheduled, Timestamp: promise.CreatedAt, Actor: domain.Actor{Type: "SYSTEM", ID: "promise-worker-v1"}, Payload: payload, CorrelationID: correlationID}}
	for _, event := range events {
		if err = insertEvent(ctx, tx, event); err != nil {
			return promise, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return promise, false, err
	}
	return promise, true, nil
}

func (p *Postgres) GetPromise(ctx context.Context, promiseID domain.ID) (domain.PromiseToPay, error) {
	result, err := scanPromise(p.pool.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE id=$1`, promiseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, recovery.ErrNotFound
	}
	return result, err
}
func (p *Postgres) ListPromises(ctx context.Context, caseID domain.ID) ([]domain.PromiseToPay, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE case_id=$1 ORDER BY created_at,id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.PromiseToPay{}
	for rows.Next() {
		promise, scanErr := scanPromise(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, promise)
	}
	return result, rows.Err()
}

func (p *Postgres) CancelPromise(ctx context.Context, promiseID domain.ID, correlationID string, now time.Time) (domain.PromiseToPay, error) {
	return p.transitionPromise(ctx, promiseID, "CANCELLED", "EXPLICIT_CANCELLATION", correlationID, "", now)
}

func (p *Postgres) ClaimDuePromise(ctx context.Context, workerID string, now time.Time, lease time.Duration) (domain.PromiseToPay, error) {
	row := p.pool.QueryRow(ctx, `WITH due AS(SELECT id FROM promise_checks WHERE status IN('PENDING','CLAIMED') AND scheduled_for<=$1 AND(status<>'CLAIMED' OR lease_expires_at<=$1) ORDER BY scheduled_for,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE promise_checks c SET status='CLAIMED',lease_owner=$2,lease_expires_at=$3,attempt_count=attempt_count+1 FROM due WHERE c.id=due.id RETURNING c.promise_id`, now, workerID, now.Add(lease))
	var promiseID domain.ID
	if err := row.Scan(&promiseID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromiseToPay{}, promises.ErrNoDuePromise
		}
		return domain.PromiseToPay{}, err
	}
	return p.GetPromise(ctx, promiseID)
}

func (p *Postgres) ResolveDuePromise(ctx context.Context, promiseID domain.ID, now time.Time) (domain.PromiseToPay, error) {
	return p.resolvePromise(ctx, promiseID, "", now)
}

func (p *Postgres) ResolvePromiseForDemo(ctx context.Context, promiseID domain.ID, outcome string, now time.Time) (domain.PromiseToPay, error) {
	if outcome != "FULFILLED" && outcome != "BROKEN" {
		return domain.PromiseToPay{}, errors.New("invalid demo promise outcome")
	}
	return p.resolvePromise(ctx, promiseID, outcome, now)
}

func (p *Postgres) resolvePromise(ctx context.Context, promiseID domain.ID, forcedOutcome string, now time.Time) (domain.PromiseToPay, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.PromiseToPay{}, err
	}
	defer tx.Rollback(ctx)
	promise, err := scanPromise(tx.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE id=$1 FOR UPDATE`, promiseID))
	if err != nil {
		return promise, err
	}
	if promise.Status != "ACTIVE" {
		return promise, recovery.ErrConflict
	}
	var caseState domain.CaseState
	var deadline time.Time
	if err = tx.QueryRow(ctx, `SELECT current_state,recovery_deadline FROM recovery_cases WHERE id=$1 FOR UPDATE`, promise.CaseID).Scan(&caseState, &deadline); err != nil {
		return promise, err
	}
	target, reason, reference := forcedOutcome, "DEMO_OUTCOME_SIMULATION", "demo-control"
	if forcedOutcome == "" {
		target, reason, reference = "BROKEN", "PAYMENT_NOT_OBSERVED", ""
	}
	if forcedOutcome == "" && caseState == domain.StateRecovered {
		target, reason, reference = "FULFILLED", "CASE_RECOVERED", "case-state"
	} else if forcedOutcome == "" && !deadline.After(now) {
		target, reason = "EXPIRED", "RECOVERY_WINDOW_EXPIRED"
	}
	resolved, err := transitionPromiseTx(ctx, tx, promise, target, reason, "promise-check:"+string(promise.ID), reference, now)
	if err != nil {
		return promise, err
	}
	if _, err = tx.Exec(ctx, `UPDATE promise_checks SET status='COMPLETED',completed_at=$2,result=$3,lease_owner=NULL,lease_expires_at=NULL WHERE promise_id=$1`, promise.ID, now, target); err != nil {
		return promise, err
	}
	if target == "BROKEN" && (caseState == domain.StateWaitingOutcome || caseState == domain.StateEscalated) {
		if _, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='REASSESSING',version=version+1,updated_at=$2 WHERE id=$1`, promise.CaseID, now); err == nil {
			_, err = tx.Exec(ctx, `UPDATE recovery_cases SET current_state='ACTION_PENDING',version=version+1,updated_at=$2 WHERE id=$1`, promise.CaseID, now)
		}
		if err != nil {
			return promise, err
		}
		sequence, sequenceErr := nextSequence(ctx, tx, promise.CaseID)
		if sequenceErr != nil {
			return promise, sequenceErr
		}
		transitions := []struct{ from, to domain.CaseState }{{caseState, domain.StateReassessing}, {domain.StateReassessing, domain.StateActionPending}}
		for index, transition := range transitions {
			payload, _ := json.Marshal(map[string]any{"from_state": transition.from, "to_state": transition.to, "reason": "BROKEN_PROMISE", "promise_id": promise.ID})
			if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: promise.CaseID, Sequence: sequence + int64(index), Type: domain.EventStateTransitioned, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "promise-engine-v1"}, Payload: payload, CorrelationID: "promise-check:" + string(promise.ID)}); err != nil {
				return promise, err
			}
		}
	}
	if target == "BROKEN" || target == "FULFILLED" {
		if err = updatePromiseReliability(ctx, tx, promise.CustomerID, now); err != nil {
			return promise, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return promise, err
	}
	return resolved, nil
}

func (p *Postgres) transitionPromise(ctx context.Context, promiseID domain.ID, target, reason, correlationID, reference string, now time.Time) (domain.PromiseToPay, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.PromiseToPay{}, err
	}
	defer tx.Rollback(ctx)
	promise, err := scanPromise(tx.QueryRow(ctx, `SELECT `+promiseColumns+` FROM promises_to_pay WHERE id=$1 FOR UPDATE`, promiseID))
	if err != nil {
		return promise, err
	}
	resolved, err := transitionPromiseTx(ctx, tx, promise, target, reason, correlationID, reference, now)
	if err != nil {
		return promise, err
	}
	if _, err = tx.Exec(ctx, `UPDATE promise_checks SET status='CANCELLED',completed_at=$2,result=$3 WHERE promise_id=$1 AND status IN('PENDING','CLAIMED')`, promiseID, now, target); err != nil {
		return promise, err
	}
	if err = tx.Commit(ctx); err != nil {
		return promise, err
	}
	return resolved, nil
}

func transitionPromiseTx(ctx context.Context, tx pgx.Tx, promise domain.PromiseToPay, target, reason, correlationID, reference string, now time.Time) (domain.PromiseToPay, error) {
	if promise.Status != "ACTIVE" {
		return promise, recovery.ErrConflict
	}
	valid := map[string]bool{"FULFILLED": true, "BROKEN": true, "EXPIRED": true, "CANCELLED": true}
	if !valid[target] {
		return promise, errors.New("invalid promise transition")
	}
	column := map[string]string{"FULFILLED": "fulfilled_at", "BROKEN": "broken_at", "EXPIRED": "expired_at", "CANCELLED": "cancelled_at"}[target]
	query := `UPDATE promises_to_pay SET status=$2,resolved_at=$3,verification_reference=$4,` + column + `=$3 WHERE id=$1`
	if _, err := tx.Exec(ctx, query, promise.ID, target, now, reference); err != nil {
		return promise, err
	}
	payload, _ := json.Marshal(map[string]any{"promise_id": promise.ID, "from_status": promise.Status, "to_status": target, "reason": reason, "verification_reference": reference})
	if _, err := tx.Exec(ctx, `INSERT INTO promise_events(id,promise_id,case_id,from_status,to_status,reason_code,payload,occurred_at,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id.New(), promise.ID, promise.CaseID, promise.Status, target, reason, payload, now, correlationID); err != nil {
		return promise, err
	}
	sequence, err := nextSequence(ctx, tx, promise.CaseID)
	if err != nil {
		return promise, err
	}
	eventType := map[string]domain.EventType{"FULFILLED": domain.EventPromiseFulfilled, "BROKEN": domain.EventPromiseBroken, "EXPIRED": domain.EventPromiseExpired, "CANCELLED": domain.EventPromiseCancelled}[target]
	if err = insertEvent(ctx, tx, domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: promise.CaseID, Sequence: sequence, Type: eventType, Timestamp: now, Actor: domain.Actor{Type: "SYSTEM", ID: "promise-engine-v1"}, Payload: payload, CorrelationID: correlationID}); err != nil {
		return promise, err
	}
	promise.Status = target
	promise.ResolvedAt = &now
	promise.VerificationReference = reference
	switch target {
	case "FULFILLED":
		promise.FulfilledAt = &now
	case "BROKEN":
		promise.BrokenAt = &now
	case "EXPIRED":
		promise.ExpiredAt = &now
	case "CANCELLED":
		promise.CancelledAt = &now
	}
	return promise, nil
}

func updatePromiseReliability(ctx context.Context, tx pgx.Tx, customerID domain.ID, now time.Time) error {
	var fulfilled, broken int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE status='FULFILLED'),COUNT(*) FILTER(WHERE status='BROKEN') FROM promises_to_pay WHERE customer_id=$1`, customerID).Scan(&fulfilled, &broken); err != nil {
		return err
	}
	reliability := float64(fulfilled+1) / float64(fulfilled+broken+2)
	var successful, failed, tenure int
	var fatigue float64
	var features json.RawMessage
	var version int
	err := tx.QueryRow(ctx, `SELECT successful_payments,failed_payments,subscription_tenure_days,fatigue_score,features,version FROM customer_recovery_profiles WHERE customer_id=$1 ORDER BY version DESC LIMIT 1`, customerID).Scan(&successful, &failed, &tenure, &fatigue, &features, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		version = 0
		features = json.RawMessage(`{}`)
	} else if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO customer_recovery_profiles(id,customer_id,successful_payments,failed_payments,subscription_tenure_days,promise_reliability,fatigue_score,features,version,updated_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id.New(), customerID, successful, failed, tenure, reliability, fatigue, features, version+1, now)
	return err
}
