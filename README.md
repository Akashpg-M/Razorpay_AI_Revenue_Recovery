# AI Revenue Recovery Agent

This repository now contains a stateful recovery domain, immutable audit history, and a deterministic synthetic evaluation environment. It remains independent of Razorpay, AI, and ML integrations at this stage.

## Services

- `backend`: Go/Gin API and PostgreSQL-backed recovery aggregate.
- `decision-service`: FastAPI health service plus deterministic synthetic simulator.
- `frontend`: Next.js business dashboard, resilience lab, and persisted recovery-case replay.
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

## Evaluation and reviewer UI

Run the corrected full evaluation and paired ablations from `decision-service`:

```powershell
python -m evaluation.full_evaluation --dataset-size 5000 --seeds 101 202 303 404 505
python -m evaluation.ablations --dataset-size 5000 --seeds 101 202 303 404 505
```

Generate the authorization and reliability matrices from `backend`:

```powershell
go run ./cmd/evaluation ../decision-service/evaluation/results
```

After `docker compose up -d --build`, open:

- `http://localhost:3000/` — operational and synthetic business dashboard.
- `http://localhost:3000/resilience` — development-only backend fault injection.
- `http://localhost:3000/recovery/{case_id}` — replay from persisted decisions and audit records.

The dashboard labels operational/test-mode data separately from synthetic evaluation. Synthetic results are reproducible simulator measurements and are not presented as production causal uplift.

This writes `train.jsonl`, `validation.jsonl`, `test.jsonl`, and `dataset_report.json`. Each row separates `observable` features from evaluation-only `_ground_truth`. Strategies and feature pipelines must never receive `_ground_truth`.

Run tests:

```powershell
cd backend
go test ./...

cd ..\decision-service
python -m unittest discover -s tests -v
```

The simulator derives every case and every action outcome from the configured seed and stable hashes, so results do not depend on evaluation order.

## Phase 5: baseline evaluation

All four baselines use the same held-out population. Only the contextual retry baseline is fitted, and its `fit` method rejects any split other than `train`.

```powershell
cd decision-service
python -m evaluation.baselines `
  --dataset simulation/data/test.jsonl `
  --train-dataset simulation/data/train.jsonl `
  --seed 42 `
  --output-dir evaluation/results
```

The strategies are no intervention/natural recovery, fixed retry or checkout link, explicit rules `rules-v1`, and the train-fitted contextual retry-only strategy. Results are written as JSON plus [a Markdown comparison](decision-service/evaluation/results/baseline_comparison.md).

## Phases 6–7: detection and Razorpay Test Mode

Normalized event endpoints:

- `POST /api/v1/detection/subscription` with `X-Event-Id`.
- `POST /api/v1/detection/checkout` with `X-Event-Id`.
- `POST /api/v1/webhooks/razorpay` with `X-Razorpay-Event-Id` and `X-Razorpay-Signature`.

Set `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, and `RAZORPAY_WEBHOOK_SECRET` in `.env`. Do not commit their values. The webhook implementation validates HMAC-SHA256 over the unmodified request body, persists signature/processing status, and suppresses duplicate provider event IDs.

The merchant, customer, latest merchant policy and optional recovery profile must exist before ingesting a provider event. Razorpay payments/subscriptions must carry the corresponding internal `merchant_id` and `customer_id` in `notes`; subscription-only events must additionally provide `amount_minor` and `currency` in notes when no payment entity is included.

Configure a Razorpay Test Mode webhook to point to:

```text
https://<public-test-host>/api/v1/webhooks/razorpay
```

Payment Link creation and payment lookup are implemented behind `integrations/razorpay.Client`. Normal automated tests use an HTTP test server. Live Test Mode is optional and requires valid credentials and a publicly reachable webhook URL.

Official references: [webhook validation](https://razorpay.com/docs/webhooks/validate-test/), [Payment Links API](https://razorpay.com/docs/api/payments/payment-links/).

## Phases 8–9: context and eligibility

```text
GET /api/v1/recovery-cases/{case_id}/context
GET /api/v1/recovery-cases/{case_id}/eligibility
```

`recovery-context-v1` contains the case, normalized diagnosis, observable customer recovery profile, merchant constraints, recent actions, active promise, timing and payment state. Hidden simulator attributes are recursively rejected.

Eligibility removes actions before scoring and reports reason codes such as `ACTIVE_PROMISE_TO_PAY`, `QUIET_HOURS`, `CUSTOMER_OPT_OUT`, `MAX_CONTACTS_REACHED`, `MAX_RETRIES_REACHED`, `MANDATE_UNAVAILABLE`, `PAYMENT_METHOD_INVALID`, `ACTION_COOLDOWN`, `RECOVERY_WINDOW_EXPIRED` and `LEAK_TYPE_INCOMPATIBLE`. Eligibility is deliberately separate from final policy authorization.

## Phase 10: model training and prediction

Train using train plus validation only:

```powershell
cd decision-service
python -m prediction.training `
  --train simulation/data/train.jsonl `
  --validation simulation/data/validation.jsonl `
  --output-dir models `
  --seed 42
```

Run the final frozen-model test evaluation separately:

```powershell
python -m prediction.evaluate `
  --artifact models/outcome_v1.joblib `
  --dataset simulation/data/test.jsonl `
  --output evaluation/results/outcome_v1_test.json
```

Prediction service endpoint:

```text
POST /api/v1/predict/outcomes
```

The Go orchestration endpoint derives context, filters eligibility, calls the Python service and transactionally stores immutable predictions plus `ACTION_PREDICTED` audit events:

```text
POST /api/v1/recovery-cases/{case_id}/predictions
```

## Phases 11-16: bounded next-best-action execution

The decision path now learns a separate observable-only natural-recovery probability, calculates incremental uplift, ranks only eligible interventions with deterministic `nba-v1` NERV, applies an explicit economic gate and fresh deterministic policy authorization, and durably schedules approved work. PostgreSQL claim leases support at-least-once worker processing and restart recovery; stable keys deduplicate local email, local retry requests, executions, and Razorpay Payment Links.

```text
POST /api/v1/recovery-cases/{case_id}/decision
POST /api/v1/recovery-cases/{case_id}/responses
GET  /api/v1/recovery-cases/{case_id}/workflow
GET  /api/v1/recovery-cases/{case_id}/events
POST /api/v1/predict/natural
```

The Docker Compose stack includes a separate durable worker. See [the Phase 11-16 implementation report](docs/PHASE_11_16_IMPLEMENTATION.md) for model metrics, equations, policy rules, execution semantics, provider limitations, held-out comparison, and exact reproduction commands.

## Phases 17-24: learning and portfolio optimization

The recovery engine now supports durable promise-to-pay checks, versioned merchant optimization profiles, evidence-based attribution, immutable feedback and model governance, validation-only calibration, portfolio prioritization, and constrained allocation.

```text
POST /api/v1/recovery-cases/{case_id}/promises
POST /api/v1/recovery-cases/{case_id}/recovery-observations
GET  /api/v1/recovery-cases/{case_id}/attributions
POST /api/v1/merchants/{merchant_id}/optimization-profiles
POST /api/v1/merchants/{merchant_id}/portfolio-priority-runs
POST /api/v1/merchants/{merchant_id}/budget-allocation-runs
POST /api/v1/model-registry/candidates
POST /api/v1/model-registry/{registry_id}/status
```

Run the complete five-seed evaluation with `python -m evaluation.full_evaluation --dataset-size 5000 --seeds 101 202 303 404 505 --output-dir evaluation/results/phase24` from `decision-service`.

See [the Phase 17-24 implementation report](docs/PHASE_17_24_IMPLEMENTATION.md) for lifecycle semantics, governance, live verification, metrics, and limitations.

See [the Phase 5–10 implementation report](docs/PHASE_5_10_IMPLEMENTATION.md) for architecture decisions, metrics and limitations.
