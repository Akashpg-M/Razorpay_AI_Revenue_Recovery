package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"revenue-recovery/backend/internal/detection"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/integrations/razorpay"
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
	err := p.pool.QueryRow(ctx, `SELECT response FROM provider_action_references WHERE action_id=$1 AND provider='razorpay' AND operation='payment_link.create'`, actionID).Scan(&raw)
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
