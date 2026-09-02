# Razorpay Test Mode integration

This integration is optional and fail-closed. Normal startup, deterministic
tests, synthetic evaluation, the Resilience Lab, dashboards, replay, operations
queue, and demo seed use the local provider by default.

## Configuration and ownership

| Variable | Backend | Worker | Decision service | Frontend |
|---|:---:|:---:|:---:|:---:|
| `RAZORPAY_KEY_ID` | Yes | Yes | No | No |
| `RAZORPAY_KEY_SECRET` | Yes | Yes | No | No |
| `RAZORPAY_WEBHOOK_SECRET` | Yes | No | No | No |
| `RAZORPAY_API_URL` | Yes | Yes | No | No |
| `RAZORPAY_WEBHOOK_PUBLIC_URL` | Yes | No | No | No |
| `PAYMENT_PROVIDER` | Yes | Yes | No | No |

`.env` is ignored by Git. `.env.example` contains empty placeholders and the
non-secret default URL only. Credentials are loaded lazily; local mode accepts
empty Razorpay credentials. Selecting `PAYMENT_PROVIDER=razorpay` makes the
worker require a complete Test Mode credential pair. Live-key prefixes are
refused before any network request.

The supported base is `https://api.razorpay.com`. The client canonicalizes a
trailing slash and an accidental trailing `/v1`, then constructs:

```text
POST https://api.razorpay.com/v1/payment_links
GET  https://api.razorpay.com/v1/payment_links/{id}
GET  https://api.razorpay.com/v1/payments/{id}
GET  https://api.razorpay.com/v1/payments?count=1
```

Tests assert that `/v1` is neither duplicated nor omitted.

## Implemented capabilities

| Capability | Status | Implementation |
|---|---|---|
| Payment Link creation | Implemented | `backend/internal/integrations/razorpay/client.go` |
| Payment Link fetch/status | Implemented | `Client.FetchPaymentLinkWithStatus` |
| Payment lookup | Implemented | `Client.FetchPayment` |
| Subscription lookup | Not implemented | Webhook normalization only |
| Webhook HMAC verification | Implemented | `VerifyWebhookSignature` in `webhook.go` |
| Webhook deduplication | Implemented | unique provider event persistence in `store/integrations.go` |
| Event normalization | Implemented | `Ingestor` in `webhook.go` |
| `payment.failed` risk creation | Implemented | failure adapter and recovery service |
| `payment_link.paid` recovery attribution | Implemented | stored-link resolver plus `attribution.Service` |
| Provider-reference persistence | Implemented | `SavePaymentLink` and `provider_action_references` |
| Provider reconciliation | Implemented for Payment Links | `PaymentLinkExecutor.Reconcile` |

Direct manual payment retry is deliberately not claimed. The Razorpay adapter
supports Payment Links; the existing local retry executor remains separate.

## Security properties

- API authentication uses HTTP Basic authentication only inside the outbound
  request. Credentials and authorization headers are never returned.
- Provider errors expose HTTP status and a sanitized error code, not the raw
  provider body.
- Webhook verification computes HMAC-SHA256 over the unmodified request body
  using `RAZORPAY_WEBHOOK_SECRET`, then compares it with
  `X-Razorpay-Signature` using a constant-time comparison.
- The webhook secret is not passed to the worker, frontend, decision service,
  or Razorpay API calls, and is not persisted in PostgreSQL.
- Duplicate valid events are suppressed by `X-Razorpay-Event-Id`.
- Invalid signatures are not persisted and cannot poison a future genuine event ID.
- Secret-value scans over application logs and a PostgreSQL data dump found no
  configured credential values.

## Safe verification

The non-mutating endpoint is:

```text
GET /api/v1/integrations/razorpay/status
```

The CLI performs authentication and, only with an explicit flag, a controlled
synthetic ₹1 Payment Link create/persist/fetch operation:

```bash
docker compose exec -T backend ./razorpay-check
docker compose exec -T backend ./razorpay-check --create-payment-link
```

Using `exec` with a relative path is Git Bash-safe and reuses the backend's
healthy Compose network. Git Bash otherwise may translate `/app/razorpay-check`
into a Windows host path before Docker receives it.

The current configured credentials authenticated in Test Mode with HTTP 200.
One Payment Link was created with HTTP 200, its provider reference was persisted,
and fetching it returned HTTP 200 with status `created`. Repeating the command
reused the persisted reference and did not create a duplicate resource. No real
customer contact data or real-money operation was used.

## Webhooks

The exact route is:

```text
POST /api/v1/webhooks/razorpay
```

Local verification passed for valid, invalid, modified, duplicate, failure, and
Payment Link paid events. The paid path resolves the stored provider reference,
records strong direct-action attribution, and moves the case to `RECOVERED`.

Configure the Dashboard in Test Mode with:

1. `https://YOUR_PUBLIC_TEST_HOST/api/v1/webhooks/razorpay`.
2. The exact same webhook secret stored locally.
3. `payment.failed` and `payment_link.paid` for the currently verified account.
4. An active webhook, followed by a real Test Mode event and delivery-log check.

Set the non-secret `RAZORPAY_WEBHOOK_PUBLIC_URL` locally to make the status
endpoint reflect that external delivery has been configured. This is a marker,
not proof of reachability.

The current free `*.shares.zrok.io` URL was also tested. Without the
`skip_zrok_interstitial` request header it returns zrok's HTML warning page even
for a JSON POST. Razorpay Dashboard delivery cannot be assumed to supply that
header, so use an interstitial-free zrok share/domain or another HTTPS tunnel
for genuine provider callbacks. The bypass header is useful only for manual
tunnel verification.

Official references: [Payment Links API](https://razorpay.com/docs/api/payments/payment-links/),
[API keys](https://razorpay.com/docs/payments/dashboard/account-settings/api-keys/),
and [webhook validation](https://razorpay.com/docs/webhooks/validate-test/).

## Known limitations

- Subscription fetch/reconciliation is not implemented.
- Payment Link edit, cancel, resend, and direct retry are not implemented.
- Manual signed delivery through zrok has been exercised; genuine Dashboard
  delivery still requires an interstitial-free public endpoint.
- The integration-status endpoint can prove outbound API authentication but
  cannot prove that the Razorpay Dashboard points to this backend.
- The synthetic Test Mode Payment Link remains a Test Mode object; it cannot
  move real money.
