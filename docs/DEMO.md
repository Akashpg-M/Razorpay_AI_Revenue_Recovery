# Demo

1. Copy `.env.example` to `.env` and run `scripts/demo-bootstrap.ps1`. It applies the deterministic, idempotent, Test Mode-only fixture through PostgreSQL's CLI; no manual edits are needed.
2. Confirm `http://localhost:8080/health/ready`, `http://localhost:8001/health/ready`, and `http://localhost:3000/api/health`.
3. Open `/` for impact, `/operations` for human review, `/observability` for durable health, and `/resilience` for development fault scenarios.
4. In Operations, approve an escalated case. Show the fresh authorization result, scheduled action, and immutable operator events in replay. Repeat with reject and a deliberately stale case version to show that no action is scheduled.
5. Run `scripts/verify.ps1`; inspect `evaluation/results/phase33/verification_summary.json`.

Use Razorpay Test Mode only. Synthetic numbers are illustrative evaluation results, not production causal claims.
