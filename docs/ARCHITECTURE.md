# Architecture

RecoverOS is a three-service product: Next.js is the recovery control plane, Go owns the stateful aggregate and all durable mutations, and FastAPI serves observable-only prediction plus offline synthetic evaluation. PostgreSQL is authoritative; Redis is an optional readiness/cache dependency and never the source of recovery truth.

```text
payment failure / checkout abandonment
  -> detection + RecoveryCase
  -> observable context + eligibility
  -> action-conditioned and natural-recovery prediction
  -> incremental NERV ranking
  -> economic gate
  -> merchant policy / human review
  -> durable schedule + leased worker
  -> local executor or Razorpay Test Payment Link
  -> signed outcome webhook
  -> attribution + feedback + RECOVERED
```

Human approval is an authorization input, not a policy bypass: approval triggers a fresh case-version, deadline, policy and economic check immediately before scheduling. Every aggregate mutation and operator decision is transactionally paired with an append-only event. Stable idempotency keys, optimistic versions, database leases, unique provider references and webhook IDs bound at-least-once processing.

The frontend reads the same persisted models used by workers and APIs. Command Center and Recoveries show operational data; Operations exposes human controls and stopping rules; Evaluation is explicitly synthetic; Reliability invokes a guarded deterministic Go harness; Observability reads runtime/provider status; Demo coordinates real development APIs and polls the authoritative replay.

Trust boundaries: provider webhooks require a raw-body HMAC before event-ID reservation; scenario/fault APIs are disabled outside development/demo/test; client payloads cannot provide hidden simulator attributes; operator identity is audited but authentication remains deployment-owned; Razorpay credentials never leave server-side configuration. See `FINAL_IMPLEMENTATION_AUDIT.md`, `POLICY_ENGINE.md`, `RELIABILITY.md`, and `SECURITY.md`.
