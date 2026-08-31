# AI Revenue Recovery Agent

This repository now contains a stateful recovery domain, immutable audit history, and a deterministic synthetic evaluation environment. It remains independent of Razorpay, AI, and ML integrations at this stage.

## Services

- `backend`: Go/Gin API and PostgreSQL-backed recovery aggregate.
- `decision-service`: FastAPI health service plus deterministic synthetic simulator.
- `frontend`: Next.js scaffold.
- `infra/migrations`: PostgreSQL schema and append-only audit protections.

## Recovery lifecycle API

The API requires the referenced merchant and customer to exist in PostgreSQL. Create them through a seed/migration fixture before creating a case.

Create a case:

```http
POST /api/v1/recovery-cases
Content-Type: application/json

{
  "leak_type": "FAILED_SUBSCRIPTION",
  "merchant_id": "merchant-demo",
  "customer_id": "customer-demo",
  "amount_at_risk_minor": 849900,
  "currency": "INR",
  "source_reference": "invoice-demo-1",
  "source_status": "failed",
  "failure_or_leak_context": {"failure_type": "INSUFFICIENT_FUNDS"},
  "customer_context_snapshot": {},
  "merchant_policy_snapshot": {},
  "recovery_deadline": "2026-09-07T12:00:00Z",
  "actor": {"type": "SYSTEM", "id": "detector"},
  "correlation_id": "demo-run-1"
}
```

Advance it using optimistic concurrency:

```http
POST /api/v1/recovery-cases/{case_id}/transitions
Content-Type: application/json

{
  "to_state": "DIAGNOSING",
  "expected_version": 1,
  "actor": {"type": "SYSTEM", "id": "recovery-engine"},
  "payload": {"reason": "diagnosis_started"},
  "correlation_id": "demo-run-1"
}
```

Read the immutable, sequence-ordered history:

```http
GET /api/v1/recovery-cases/{case_id}/events
```

Decision and outcome components can append one of the closed event types with:

```http
POST /api/v1/recovery-cases/{case_id}/events
```

Terminal states (`RECOVERED`, `EXHAUSTED`, `STOPPED`) reject all later transitions. Each transition checks the expected aggregate version and persists the state update and audit event in one transaction.

## Synthetic simulator

Generate the default 5,000-case population:

```powershell
cd decision-service
python -m simulation.generate --seed 42 --dataset-size 5000 --output-dir simulation/data
```

`--merchant-mix`, `--subscription-failure-distribution`, and
`--checkout-failure-distribution` accept either an inline JSON object or the path
to a JSON file. All weights must be non-negative and sum to `1.0`.

This writes `train.jsonl`, `validation.jsonl`, `test.jsonl`, and `dataset_report.json`. Each row separates `observable` features from evaluation-only `_ground_truth`. Strategies and feature pipelines must never receive `_ground_truth`.

Run tests:

```powershell
cd backend
go test ./...

cd ..\decision-service
python -m unittest discover -s tests -v
```

The simulator derives every case and every action outcome from the configured seed and stable hashes, so results do not depend on evaluation order.
