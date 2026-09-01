# Phases 11–16 implementation report

## 1. Files added or modified

Phase 11 adds `prediction/natural_training.py`, `natural_service.py`, `natural_evaluate.py`, the natural model artifact/metadata, an HTTP prediction route, and regression/leakage tests. Phases 12–14 add the Go `optimizer`, `decisioning`, `economicgate`, and `policy` packages plus append-only persistence. Phases 15–16 add `orchestrator`, `executor`, `responses`, the worker command, workflow/response APIs, PostgreSQL repositories, Docker worker wiring, and tests. `evaluation/nba.py` and its result files provide the frozen held-out comparison. Migration `000005_bounded_recovery_workflow` owns all new durable records.

## 2. Migration

`000005_bounded_recovery_workflow.up.sql` adds immutable natural predictions, immutable decisions/candidates, economic gates, policy evaluations, leased scheduled actions, execution idempotency/failure fields, local email captures, retry-request captures, and normalized customer responses. `000006_execution_replay_fix` changes the execution idempotency index from partial to full after live PostgreSQL verification exposed an incompatible `ON CONFLICT` inference. Matching down migrations reverse the additions. Existing pre-Phase-11 migration files were not edited.

## 3. Natural-recovery model

`WAIT` is the sole canonical representation of no immediate intervention for the current decision horizon. There are no duplicate `NO_ACTION` or `DO_NOTHING` actions.

The model is fitted only on observable WAIT examples. Logistic Regression and Gradient Boosting were trained on 3,500 training cases. The first half of validation was available for sigmoid calibration and the second half for candidate selection. Calibration is accepted per candidate only when it improves Brier score; it was rejected for the selected Gradient Boosting model because Brier worsened from `0.174361` to `0.175762` (ECE `0.010293` to `0.032558`). Validation metrics for the selected uncalibrated model are ROC-AUC `0.573266`, Brier `0.174361`, ECE `0.010293`, and average precision `0.272662`.

Frozen held-out metrics are ROC-AUC `0.537887`, Brier `0.169173`, ECE `0.031537`, and average precision `0.263388`. Versions are `natural-recovery-v1` and `natural-features-v1`. Artifact SHA-256 is `F60830F98F1EADC27017A085E949AE982D6B1F2B74B6D19E1A2D174941C55C09`. The test split hash remains `CC1A248AF3E1026761DC60CED0394FD89FD784646DC73F659AEC69E2BBB763F2`.

## 4. Counterfactual uplift contract

For one case snapshot, natural recovery is predicted exactly once and reused for all modeled eligible candidates:

`incremental_uplift = P(recovery | observable context, action) - P(recovery | observable context, WAIT)`

Negative values are retained. Every immutable snapshot records case/version, context version, both model versions, both probabilities, uplift, candidate scores, and timestamps. A row lock plus case-version comparison rejects stale writes.

## 5. NBA and cost model

The deterministic `nba-v1` optimizer computes:

`NERV = uplift × amount_at_risk - channel_cost - incentive_cost - operational_cost - fatigue_penalty - risk_penalty`

Currency outputs are `int64` minor units. Probability is converted to fixed millionths and multiplication uses `math/big`, avoiding floating-point persisted money. `cost-v1` explicitly versions email/local channel, payment-link operations, retry operations, retention incentive, fatigue, and risk penalties. Objectives share one framework: `MAXIMIZE_NET_RECOVERY`, `MAXIMIZE_RETENTION`, `MINIMIZE_CONTACT`, `MINIMIZE_RECOVERY_COST`, and `BALANCED`. Ranking ties resolve by objective score, then NERV, then lexical action name.

## 6. Worked candidate example

For ₹8,000 at risk, natural recovery `0.35`, diagnosis confidence `0.90`, and low fatigue:

| Candidate | Action probability | Uplift | Gross incremental (minor) | Costs/penalties (minor) | NERV (minor) |
|---|---:|---:|---:|---:|---:|
| RETRY_LATER | 0.64 | +0.29 | 232000 | 435 | 231565 |
| SEND_REMINDER | 0.30 | -0.05 | -40000 | at least 25 | negative |
| WAIT | 0.35 | 0 | 0 | 0 | 0 |

`RETRY_LATER` ranks first, the zero-threshold economic gate returns `ALLOW`, and policy returns `APPROVE` when limits/current state permit. It is durably scheduled; the worker later reloads the case and repeats policy authorization. A successful retry request enters `WAITING_OUTCOME`; an unresolved 24-hour observation creates `REASSESSING → ACTION_PENDING` and a new NBA decision.

## 7. Economic gate

`economic-gate-v1` is intentionally separate from ranking. NERV above or exactly at the merchant threshold is allowed. NERV below threshold is `BLOCK/BELOW_MINIMUM_VALUE`; negative NERV is `BLOCK/ECONOMICALLY_UNJUSTIFIED`. Missing configuration defaults to zero. WAIT is allowed as the no-spend choice. Gate inputs and results are append-only and audited.

## 8. Policy rules and precedence

Precedence is `STOP > DENY > ESCALATE > APPROVE`; all matching reason codes within the winning level are retained.

| Result | Rules/checks |
|---|---|
| STOP | payment already recovered, terminal state, subscription cancelled, recovery window expired |
| DENY | stale decision, economic block, customer opt-out, max retries, known permanent failure, retry cooldown, daily/weekly contact limits, minimum contact interval, active PTP, invalid/revoked mandate, invalid payment method, maximum incentive, quiet hours in merchant IANA timezone, unavailable channel |
| ESCALATE | high-value approval threshold, low diagnosis confidence |
| APPROVE | all checks passed |

Eligibility removes incompatible/unscoreable actions first. Policy then authorizes the selected action from current state. `ESCALATE` is persisted and moves the case to durable `ESCALATED`; it never schedules automatically. Every result has a specific audit event and append-only evaluation.

## 9. Orchestrator flow

The path is `ACTION_PENDING → POLICY_REVIEW → SCHEDULED → EXECUTING → WAITING_OUTCOME`. Success creates a durable observation deadline. Recovery webhooks can complete the existing recovery flow; otherwise the observer moves `WAITING_OUTCOME → REASSESSING → ACTION_PENDING` and invokes a new decision. Permanent execution failures do the same immediately. Expired cases become `EXHAUSTED`.

`GET /api/v1/recovery-cases/:id/workflow` exposes current case state, latest decision/policy evaluation, scheduled actions, and execution history. Existing `/events` exposes the complete audit trail. Customer responses enter through `POST /api/v1/recovery-cases/:id/responses` and normalize acknowledgement, intent-to-pay, PTP, opt-out, method issue, and unresolved outcomes.

## 10. Concurrency and idempotency

PostgreSQL is authoritative. Claims use `FOR UPDATE SKIP LOCKED`, lease owner/expiry, and retry state. Expired claims can be reclaimed after a worker crash. Transient failures use bounded exponential backoff and a maximum attempt count. Execution is explicitly at-least-once, not exactly-once. Stable scheduled-action keys deduplicate local email, retry capture, executions, and Razorpay payment-link references. Redis loss cannot erase pending work because Redis is not required by the worker.

## 11. Execution channels

- Local email capture: reminder, payment-method update, checkout recovery, and alternate-method templates. Payloads whitelist customer-safe fields and exclude scores/internal policy details.
- Razorpay Payment Link adapter: create/reuse by durable scheduled action reference, persist provider reference, and rely on the existing status adapter/webhook for outcome observation.
- Local retry-request capture: a durable development outbox used by RETRY_NOW/RETRY_LATER. It deliberately does not claim a Razorpay manual-retry API exists. A real supported retry workflow can replace this provider through the same interface.

## 12. Razorpay verification boundary

HTTP behavior, payment lookup, webhook signature handling, and payment-link idempotent reuse are verified with local HTTP/unit tests. No live Razorpay Test Mode credentials were used in this run, so live provider behavior is not claimed. Direct manual subscription retry is not claimed or invented. Local email and retry captures are clearly identified as local adapters.

## 13–14. Held-out scenario and baseline output

A live local-container case produced natural probability `0.22076675`, RETRY_LATER probability `0.49337756`, uplift `0.27261080`, NERV `231233` minor, gate `ALLOW`, and policy `APPROVE`. RETRY_LATER was first persisted 24 hours in the future, demonstrating delayed scheduling. Its due time was advanced only for the test. A deliberately induced persistence error stranded the record in `EXECUTING`; after migration/fix and worker restart, the expired lease was reclaimed. The final state was `WAITING_OUTCOME`, scheduled status `OBSERVATION_PENDING`, execution status `OUTCOME_PENDING`, and provider `local-retry-capture`. PostgreSQL showed two worker attempts, one external retry capture, and one persisted execution. Advancing the observation deadline created a second immutable NBA decision and `OUTCOME_OBSERVED`; current retry cooldown then denied immediate repetition. This validates at-least-once replay and deduplication without claiming exactly-once delivery.

On the unchanged 750-case synthetic test population, intermediate NBA produced gross recovered value `45,279,838` minor, intervention cost `133,515`, net recovered value `45,146,323`, recovery rate `40.4%`, 266 attempts, 353 contacts, mean recovered latency `27.5116h`, and 131 WAIT decisions. It encountered 2,560 negative-uplift candidates and selected none of them. Economic blocks, policy denials, and escalations are zero in this dataset because merchant economic thresholds/channel policy are absent from its observable schema; those boundaries are covered in Go tests.

| Strategy | Gross recovered (minor) | Interventions |
|---|---:|---:|
| No Recovery | 23934707 | 0 |
| Fixed Retry | 42608073 | 750 |
| Rules v1 | 41892917 | 667 |
| Contextual Retry-Only | 34009174 | 277 |
| Intermediate NBA | 45279838 | 619 |

This is synthetic intermediate evaluation, not final causal attribution or a production-superiority claim. Incremental attributed recovered value is intentionally `null` until the later attribution phase.

## 15. Verification performed

- `go test ./...`: all backend packages passed.
- `go vet ./...`: passed.
- `go build ./cmd/api` and `go build ./cmd/worker`: passed.
- Python `unittest discover`: 28 tests passed, including leakage, split isolation, deterministic fixtures, NBA arithmetic, WAIT, and negative uplift.
- Natural training, frozen held-out natural evaluation, and frozen NBA evaluation completed.
- `docker compose config`: passed.
- Live Docker build/start: backend and decision service healthy, worker running, frontend health endpoint `200`, PostgreSQL and Redis healthy.
- Live migrations 000002–000006: passed; schema metadata is `phase_16.1`, required tables/indexes were inspected with PostgreSQL.

## 16. Known limitations

The natural model has modest discrimination and should be improved with real observable payment history. Synthetic outcomes cannot establish causal production uplift. Payment-link expiry is bounded by the recovery case/observation workflow but is not yet sent as a provider-side expiry field. Local email/retry captures are development adapters. Human approval has a durable escalated state and inspection interface, but no operations UI. Provider reconciliation is intentionally minimal. Customer-triggered immediate reassessment is recorded; timeout-driven reassessment is the fully automatic durable path.

## 17. Reproduction commands

From `backend`:

```powershell
$env:GOFLAGS='-buildvcs=false'
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/api
go build ./cmd/worker
```

From `decision-service`:

```powershell
.\.venv\Scripts\python.exe -m prediction.natural_training --train simulation/data/train.jsonl --validation simulation/data/validation.jsonl --output-dir models --seed 42
.\.venv\Scripts\python.exe -m prediction.natural_evaluate --artifact models/natural_recovery_v1.joblib --dataset simulation/data/test.jsonl --output evaluation/results/natural_recovery_v1_test.json
.\.venv\Scripts\python.exe -m evaluation.nba --dataset simulation/data/test.jsonl --outcome-artifact models/outcome_v1.joblib --natural-artifact models/natural_recovery_v1.joblib --baselines evaluation/results/baseline_comparison.json --output evaluation/results/nba_intermediate.json --seed 42
.\.venv\Scripts\python.exe -m unittest discover -s tests -v
```

From the repository root: `docker compose config`; when Docker is running, start PostgreSQL/Redis and run the migration container before the API/worker.

## 18. Implementation issues resolved

Sigmoid calibration degraded the selected model, so training now compares calibrated and raw Brier scores and retains the better form. The sandbox could not access the user-owned Python interpreter without explicit permission; the existing project environment was then used. The scheduler stores every state transition rather than jumping directly from action-pending to scheduled. Direct retry capability was initially unavailable; it is represented honestly as a replaceable local retry-request outbox instead of a fabricated Razorpay endpoint. A live cold-start inference took 4.86 seconds and exposed an overly aggressive two-second HTTP timeout; the model-client timeout was raised to ten seconds while request contexts retain caller-level deadlines. The first worker probe also showed that the currently scheduled retry was being counted as its own historical retry and triggering cooldown; decision context now excludes `SCHEDULED` actions from completed action history. The next probe exposed PostgreSQL conflict inference against a partial unique index and an unreclaimable `EXECUTING` lease; migration 000006 creates an inferable full unique index and the claim query now reclaims expired `EXECUTING` leases. Finally, observation reassessment showed that retry cooldown existed only in final policy, allowing a futile re-score; eligibility now removes retry actions in cooldown before prediction.

## 19. Phase 17+

Next work should add dedicated promise-to-pay lifecycle automation, stronger provider reconciliation, human approval UI/workflow, causal attribution, merchant budgets, live email providers, production observability/security review, and online model monitoring/retraining. Those later capabilities must keep the same probability → optimizer → gate → policy → orchestrator → executor boundary.
