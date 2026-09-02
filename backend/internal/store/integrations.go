package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/integrations/razorpay"
	"revenue-recovery/backend/internal/recovery"
)

func (p *Postgres) InsertWebhook(ctx context.Context, w razorpay.WebhookRecord) (bool, error) {
	command, err := p.pool.Exec(ctx, `INSERT INTO webhook_events
		(id,provider,provider_event_id,event_type,payload,status,signature_status,provider_references,received_at)
		VALUES($1,'razorpay',$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(provider,provider_event_id) DO NOTHING`,
		w.ID, w.ProviderEventID, w.EventType, w.Payload, w.ProcessingStatus, w.SignatureStatus, w.ProviderReferences, w.ReceivedAt)
	return command.RowsAffected() == 1, err
}
func (p *Postgres) MarkWebhookProcessed(ctx context.Context, eventID, status string, processed time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE webhook_events SET status=$1,processed_at=$2 WHERE provider='razorpay' AND provider_event_id=$3`, status, processed, eventID)
	return err
}
func (p *Postgres) UpsertCheckout(ctx context.Context, c detection.CheckoutSession) (detection.CheckoutSession, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO checkout_sessions(checkout_id,merchant_id,customer_id,amount_minor,currency,stage,payment_method,valid_until)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(checkout_id) DO UPDATE SET stage=EXCLUDED.stage,
		payment_method=EXCLUDED.payment_method,valid_until=EXCLUDED.valid_until,updated_at=NOW()`, c.CheckoutID, c.MerchantID, c.CustomerID, c.AmountMinor, c.Currency, c.Stage, c.PaymentMethod, c.ValidUntil)
	return c, err
}
func (p *Postgres) GetCheckout(ctx context.Context, checkoutID string) (detection.CheckoutSession, error) {
	var c detection.CheckoutSession
	err := p.pool.QueryRow(ctx, `SELECT checkout_id,merchant_id,customer_id,amount_minor,currency,stage,payment_method,valid_until FROM checkout_sessions WHERE checkout_id=$1`, checkoutID).
		Scan(&c.CheckoutID, &c.MerchantID, &c.CustomerID, &c.AmountMinor, &c.Currency, &c.Stage, &c.PaymentMethod, &c.ValidUntil)
	return c, err
}
func (p *Postgres) GetPaymentLink(ctx context.Context, actionID string) (razorpay.PaymentLink, bool, error) {
	var raw []byte
	err := p.pool.QueryRow(ctx, `SELECT r.response FROM provider_action_references r JOIN recovery_actions a ON a.id=r.action_id WHERE (r.action_id=$1 OR a.idempotency_key=$1) AND r.provider='razorpay' AND r.operation='payment_link.create'`, actionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return razorpay.PaymentLink{}, false, nil
	}
	if err != nil {
		return razorpay.PaymentLink{}, false, err
	}
	var link razorpay.PaymentLink
	if err = json.Unmarshal(raw, &link); err != nil {
		return link, false, err
	}
	return link, true, nil
}
func (p *Postgres) SavePaymentLink(ctx context.Context, actionID string, link razorpay.PaymentLink, raw json.RawMessage) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO provider_action_references(id,action_id,provider,operation,provider_reference,response)
		VALUES($1,$2,'razorpay','payment_link.create',$3,$4) ON CONFLICT(action_id,provider,operation) DO NOTHING`, id.New(), actionID, link.ID, raw)
	return err
}

// ResolvePaymentLinkCase uses server-side execution data first. Provider notes
// are a signed correlation fallback, never the primary source of truth.
func (p *Postgres) ResolvePaymentLinkCase(ctx context.Context, linkID, referenceID string, noteCaseID, merchantID, customerID domain.ID) (domain.ID, error) {
	var caseID domain.ID
	err := p.pool.QueryRow(ctx, `SELECT a.case_id FROM provider_action_references r
		JOIN recovery_actions a ON a.id=r.action_id
		WHERE r.provider='razorpay' AND r.operation='payment_link.create' AND r.provider_reference=$1
		ORDER BY r.created_at DESC LIMIT 1`, linkID).Scan(&caseID)
	if err == nil {
		return caseID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if referenceID != "" {
		err = p.pool.QueryRow(ctx, `SELECT case_id FROM scheduled_actions WHERE id=$1`, referenceID).Scan(&caseID)
		if err == nil {
			return caseID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if noteCaseID == "" {
		return "", recovery.ErrNotFound
	}
	var storedMerchant, storedCustomer domain.ID
	err = p.pool.QueryRow(ctx, `SELECT merchant_id,customer_id FROM recovery_cases WHERE id=$1`, noteCaseID).Scan(&storedMerchant, &storedCustomer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", recovery.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if (merchantID != "" && merchantID != storedMerchant) || (customerID != "" && customerID != storedCustomer) {
		return "", errors.New("Razorpay payment link notes do not match the recovery case")
	}
	return noteCaseID, nil
}
