# Architecture

RecoverOS is a three-service system: Next.js presents operational evidence, Go owns the recovery aggregate and all durable mutations, and FastAPI performs observable-only prediction. PostgreSQL is authoritative; Redis is optional infrastructure and never the source of recovery truth.

The path is detection → context → eligibility → prediction → NERV optimizer → economic gate → policy authorization → durable schedule → execution → observation → attribution. Human approval is another authorization input, not a policy bypass. Every mutation and operator decision is transactionally paired with an append-only event. Stable idempotency keys, optimistic case versions, leases, and execution-time reauthorization bound at-least-once processing.

Trust boundaries: provider webhooks require signatures; client payloads cannot provide hidden simulator attributes; operators are recorded but authentication remains deployment-owned; Razorpay credentials stay server-side. See `POLICY_ENGINE.md`, `RELIABILITY.md`, and `SECURITY.md`.
