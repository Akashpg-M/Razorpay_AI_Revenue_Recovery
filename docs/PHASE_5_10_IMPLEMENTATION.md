# Phases 5–10 implementation report

## Implemented

### Phase 5

- Four deterministic, versioned strategies evaluated against the same held-out cases.
- Natural recovery is retained in the no-intervention control.
- Contextual retry is fitted only on the training split and restricted to retry timing, wait and stop.
- Aggregate, per-vertical, action and failure metrics are emitted as JSON and Markdown.

### Phase 6

- Subscription adapter for normalized payment/subscription/mandate failures.
- Stateful checkout adapter for started, method-selected, payment-failed and abandoned events.
- Both adapters produce `NormalizedLeak`, then the same canonical `RecoveryCase`.
- Provider event IDs and source references are preserved; source uniqueness makes detection idempotent.

### Phase 7

- Raw-body HMAC-SHA256 webhook verification.
- Durable webhook signature/processing metadata and duplicate suppression.
- Razorpay payload normalization is isolated from the recovery domain.
- Test-mode Payment Link creation and payment lookup HTTP adapters.
- Payment Link executions reuse a persisted provider reference for the same action.

### Phase 8

- Stable `recovery-context-v1` decision contract.
- Deterministic diagnosis categories, recoverability and confidence.
- Observable customer, merchant, action, promise, timing and payment context.
- Recursive rejection of simulator-only hidden feature names.

### Phase 9

- Deterministic pre-scoring eligibility with explainable exclusions.
- Table-driven coverage for opt-out, quiet hours, channels, PTP, contacts, retries, mandate/payment state, cooldown, deadline and vertical compatibility.
- Integration test proves prohibited actions never cross the predictor boundary.

### Phase 10

- Versioned `features-v1` action-conditioned feature pipeline.
- Logistic regression and gradient boosting candidates.
- Train-only fitting, first validation half for sigmoid calibration, second validation half for selection.
- Frozen `outcome-v1` artifact and metadata.
- Strict FastAPI prediction schema and typed Go client.
- Durable, append-only prediction records and `ACTION_PREDICTED` events.

## Architecture decisions

- Go remains responsible for detection, state, context orchestration, eligibility and persistence.
- Python remains responsible for features, fitting, calibration, evaluation and inference.
- Provider payloads terminate at adapters; downstream intelligence sees only the canonical domain/context contracts.
- `STOP` and `ESCALATE_TO_HUMAN` are control actions and are not sent to the learned outcome model.
- Model selection prioritizes Brier score, then calibration error, then validation business-ranking value.
- Held-out potential outcomes are consumed only by final evaluation, never fitting or model selection.

## Baseline held-out results

Dataset: 750 cases, ₹12,12,899.24 at risk, SHA-256 `CC1A248AF3E1026761DC60CED0394FD89FD784646DC73F659AEC69E2BBB763F2`.

| Baseline | Recovered | Recovery rate | Attempts | Contacts | Mean latency |
|---|---:|---:|---:|---:|---:|
| No recovery | ₹2,39,347.07 | 21.60% | 0 | 0 | 50.17 h |
| Fixed retry/link | ₹4,26,080.73 | 34.80% | 515 | 235 | 28.29 h |
| Rules v1 | ₹4,18,929.17 | 35.47% | 253 | 414 | 27.10 h |
| Contextual retry-only | ₹3,40,091.74 | 26.53% | 277 | 0 | 41.91 h |

These are simulator measurements, not production claims. The contextual retry aggregate includes checkout cases where retry-only deliberately waits; compare its subscription metrics separately when evaluating retry quality.

## Model selection results

Selection uses only the second half of validation after calibration on the first half.

| Model | ROC-AUC | Brier | ECE | Avg precision | Simple ranker net value |
|---|---:|---:|---:|---:|---:|
| Logistic regression | 0.596838 | 0.181605 | 0.021009 | 0.323401 | ₹2,20,167.87 |
| Gradient boosting | 0.633888 | 0.175082 | 0.022627 | 0.401176 | ₹2,81,965.26 |

Chosen: calibrated gradient boosting, model `outcome-v1`, feature contract `features-v1`.

For gradient boosting, sigmoid calibration improved validation Brier score from `0.175993` to `0.175082` and expected calibration error from `0.024255` to `0.022627`. Logistic-regression calibration did not improve its validation Brier score, which further supported selecting gradient boosting.

Frozen held-out model result: ROC-AUC `0.622492`, Brier `0.175677`, ECE `0.020511`, average precision `0.347985`. The simple action ranker realized ₹4,70,884.99 net in the evaluation environment. This is not the final Phase 12 NBA optimizer.

## Commands

Run the complete reproducible evaluation from `decision-service`:

```powershell
python -m evaluation.baselines --dataset simulation/data/test.jsonl --train-dataset simulation/data/train.jsonl --seed 42 --output-dir evaluation/results
python -m prediction.training --train simulation/data/train.jsonl --validation simulation/data/validation.jsonl --output-dir models --seed 42
python -m prediction.evaluate --artifact models/outcome_v1.joblib --dataset simulation/data/test.jsonl --output evaluation/results/outcome_v1_test.json
python -m unittest discover -s tests -v
```

From `backend`:

```powershell
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/api
```

## Tests

- State/domain lifecycle and audit tests.
- Baseline determinism, identical population, train/test isolation and action-restriction tests.
- Detection normalization, malformed input and duplicate-case tests.
- Webhook valid/invalid signature, malformed payload and duplicate delivery tests.
- Provider error, payment lookup and Payment Link idempotency tests.
- Diagnosis, context differentiation, determinism and hidden-feature isolation tests.
- Eligibility table and prediction-boundary tests.
- Artifact loading, strict schemas, probability bounds, one-result-per-action and frozen-fixture tests.

## Known limitations

- Razorpay Test Mode was implemented and mock-tested, but no live credentialed call or public webhook delivery was executed in this environment.
- Provider-to-internal merchant/customer mapping currently uses Razorpay `notes`; a production deployment should use a dedicated provider reference mapping table and onboarding flow.
- Concurrent first-time Payment Link calls require a durable reservation/worker lease for stronger side-effect serialization; repeated completed executions are idempotent today.
- Quiet hours currently interpret configured clock times in the service clock zone; merchant IANA timezone support remains for the final policy phase.
- Checkout demo state is durable, but inventory revalidation remains an external merchant adapter responsibility.
- The simulator is synthetic. Reported values demonstrate reproducible behavior, not expected production recovery rates.
- PostgreSQL migrations require live-stack verification when Docker/PostgreSQL is available.

## Phase 11+

The next stages should add natural-recovery prediction where separately modeled, incremental uplift/economic gating, the final deterministic policy engine, NBA value selection, execution workers, attribution, resilience tests and portfolio evaluation. No final NBA superiority claim is made here.
