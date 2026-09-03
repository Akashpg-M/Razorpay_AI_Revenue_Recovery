# RecoverOS Final Implementation Audit

Audit date: 2026-09-02  
Schema: `phase_55`
Claim scope: PostgreSQL-backed operational flows, Razorpay Test Mode, and separately labelled synthetic evaluation.

## Current Razorpay capability

`PAYMENT_PROVIDER=razorpay` is fail-closed: the worker starts only with configured Test Mode credentials. The status endpoint (`GET /api/v1/integrations/razorpay/status`) reports configuration, authentication, reachability and supported lookups without returning secrets. The verified environment returned authenticated Test Mode status and the configured Cloudflare tunnel returned backend readiness.

The primary success path is:

1. `executor.PaymentLinkExecutor.Execute` adds merchant, customer, recovery-case and recovery-action identifiers to Payment Link notes and uses the durable schedule ID as `reference_id`.
2. `razorpay.PaymentLinkExecutor.Execute` checks `provider_action_references`, creates the Test Mode link, then persists the provider reference and safe response.
3. `api.Detection.razorpayWebhook` reads the raw body and Razorpay headers.
4. `razorpay.Ingestor.Ingest` verifies HMAC-SHA256 before reserving the event ID, decodes the envelope and inserts a unique `webhook_events` row.
5. `payment_link.paid` is handled by `observePaymentLinkPaid`. Resolution uses, in order, the persisted Payment Link/reference mapping and validated note identifiers through `store.ResolvePaymentLinkCase`.
6. `attribution.Service.Observe` calls `store.AttributeRecovery`, which links the payment to the execution/action/decision, inserts `recovery_attributions`, fulfils a matching promise where applicable, records feedback, emits audit events and changes the case to `RECOVERED` in one transaction.

Key tables are `recovery_cases`, `recovery_events`, `recovery_decisions`, `recovery_decision_candidates`, `economic_gate_evaluations`, `policy_evaluations`, `recovery_actions`, `scheduled_actions`, `executions`, `provider_action_references`, `webhook_events`, `recovery_attributions`, `feedback_records`, `human_review_records`, and `promises_to_pay`. Schema `phase_55` additionally freezes the observable decision context and full eligibility snapshot on each decision so later replay is not reconstructed from mutable profiles.

## Razorpay webhook allowlist

Operationally handled:

- `payment.failed`: normalized into subscription-style revenue-risk detection when merchant/customer notes are present.
- `payment_link.paid`: treated as a successful recovery observation and automatically attributed.
- Legacy subscription/mandate failure names remain normalizable in code, but are not claimed as dashboard-configurable for this Razorpay account.

Intentionally not handled as recovery success: `payment.authorized`, `payment.captured`, `order.paid`, and invoice events. The demonstrated success contract is specifically `payment_link.paid`; broad payment-success matching could misattribute unrelated payments.

## Gap classification

| Item | Status | Final behavior |
|---|---|---|
| Outbound Payment Link mapping metadata | FIXED | Link notes contain case/action/merchant/customer IDs; `reference_id` contains schedule ID. |
| `payment_link.paid` support | FIXED | Signed event resolves the case and invokes automatic observation. |
| `payment.captured` support | INTENTIONALLY OUT OF SCOPE | Not safely attributable to a RecoveryCase without a narrower contract. |
| Exact payment/link/action/case correlation | FIXED | Persisted provider reference plus link/reference/note matching; exact execution reference has highest attribution precedence. |
| Automatic attribution | FIXED | No manual observation is required for the Payment Link path. |
| Duplicate success-event idempotency | FIXED | `(provider, provider_event_id)` and `(case_id, payment_reference)` uniqueness prevent repeated effects. |
| Invalid-signature dedupe poisoning | FIXED | Signature verification happens before event-ID insertion. |
| Payment Link status interpretation | PARTIAL | Fetch/reconciliation is supported; webhook outcome remains authoritative and all provider statuses are not mapped into domain outcomes. |
| Response-loss reconciliation | PARTIAL | A persisted link can be fetched and duplicate effects are bounded; a network loss before the newly created reference is persisted cannot always be discovered from Razorpay. |
| `WAIT_FOR_PROMISE_TO_PAY` executor | FIXED AS CONTROL SEMANTIC | It is not model-scoreable/schedulable. An active PTP makes interventions ineligible and `WAIT` is the executable-free outcome. |
| `RETENTION_ACTION` executor | FIXED AS NON-EXECUTABLE | Removed from executable prediction/optimization until a real channel exists. |
| Provider-mode frontend label | FIXED | Navigation and Observability call the real status API and label Razorpay Test Mode separately. |

## Executable action matrix

| Action | Runtime |
|---|---|
| `RETRY_NOW`, `RETRY_LATER` | Durable retry executor backed by the configured repository retry provider; not claimed as a Razorpay subscription retry API. |
| `SEND_REMINDER`, `REQUEST_PAYMENT_METHOD_UPDATE`, `SEND_CHECKOUT_RECOVERY_LINK`, `SUGGEST_ALTERNATE_METHOD` | Safe local email-capture executor used for deterministic demonstration. |
| `SEND_PAYMENT_LINK` | Razorpay Test Mode Payment Link executor when provider mode is `razorpay`; deterministic local link executor in local mode. |
| `WAIT` | Optimizer baseline/no-intervention outcome; never scheduled. |
| `WAIT_FOR_PROMISE_TO_PAY`, `RETENTION_ACTION`, `ESCALATE_TO_HUMAN`, `STOP` | Policy/control semantics; excluded from executable scoring. |

## Safety and durability

- Terminal `RECOVERED`, `EXHAUSTED`, and `STOPPED` cases reject attribution mutations, preventing late events from reviving or double-counting a case. A valid late provider event is acknowledged and stored as processed with outcome `IGNORED_TERMINAL_CASE`, avoiding a retry storm.
- Active PTP, recovery deadline, customer opt-out, quiet hours, contact/retry caps, invalid mandate/payment method, economic gate and merchant policy produce machine-readable reason codes.
- Human approval is followed by fresh case/policy/deadline/economic reauthorization.
- PostgreSQL schedules, stable idempotency keys, leases and provider-reference reconciliation bound at-least-once execution. Redis is not authoritative.
- The development-only Scenario and Reliability APIs are unavailable outside `development`, `demo`, or `test`.

## Safe demo claims

- RecoverOS creates real PostgreSQL cases from payment failures and checkout abandonment.
- It compares action-conditioned recovery against natural recovery, ranks by incremental NERV, applies an economic gate and merchant policy, and records model/policy provenance.
- A human-approved high-value case creates a real Razorpay Test Mode Payment Link and can automatically recover through a genuine signed `payment_link.paid` webhook.
- Synthetic held-out evaluation compares RecoverOS with baselines; those values are not production merchant revenue.
- Reliability Lab results are deterministic domain/worker simulations unless explicitly identified as a live tunnel or Test Mode call.

## Unsupported claims and remaining limitations

- No Live Mode money movement, production causal-uplift result, Razorpay subscription retry API, SMS/voice/email delivery provider, or universal Razorpay success-event support is claimed.
- Cloudflare quick-tunnel URLs are temporary and must remain running during a demo.
- Completing the hosted Razorpay checkout is an external user action. Until it occurs and Razorpay sends `payment_link.paid`, the verified case correctly remains `WAITING_OUTCOME`.
- Response-loss recovery before provider-reference persistence and full Payment Link status semantics remain documented gaps, not hidden guarantees.
