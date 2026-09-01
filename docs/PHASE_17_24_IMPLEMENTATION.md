# Phases 17-24 implementation report

## 1. Scope

Phases 17 through 24 build on the bounded decision and execution system from Phases 11-16. Hidden simulator state never reaches a decision, predictions cannot bypass policy, and portfolio allocation remains advisory until execution performs a fresh authorization check.

## 2. Architecture decisions

Durable work uses PostgreSQL claim leases instead of sleeps. Money remains integer minor units and weights use basis points. Business records are versioned or append-only; model activation requires an explicit reviewed transition.

## 3. Database migration

Migration `000007_revenue_recovery_platform` adds promise checks/events, merchant profiles, attribution rules/records, feedback, dataset manifests, model registry entries/status events, priority snapshots, and allocation runs. Append-only triggers reject updates and deletes. The live schema reports migration `7`, dirty `false`, and metadata `phase_24`.

## 4. Phase 17 promise model

Promises support `ACTIVE`, `FULFILLED`, `BROKEN`, `EXPIRED`, and `CANCELLED`. Records include amount, due time, source response, confidence, extractor version/timestamp, resolution timestamps, and verification reference.

## 5. Deterministic extraction

`ptp-extractor-v1` accepts explicit timestamps, ISO dates, “tomorrow”, and named weekdays in an IANA timezone. Ambiguous, past, low-confidence, and non-positive inputs fail before persistence. Source response IDs uniquely deduplicate response-derived promises.

## 6. Durable checking

Creating a promise atomically creates its append-only event, case audit events, and a `promise_checks` row. The worker claims due checks with `FOR UPDATE SKIP LOCKED`, an identity, and an expiring lease. Restarted workers reclaim abandoned checks.

## 7. Outcomes and reassessment

A recovered case fulfills a promise; an elapsed recovery window expires it; otherwise it breaks. A broken promise moves `WAITING_OUTCOME` or `ESCALATED` through `REASSESSING` to `ACTION_PENDING`, audits both transitions, updates versioned customer reliability, and invokes normal decision/scheduling.

## 8. Promise APIs and audit

The API supports create/list/get/cancel. Customer `PROMISE_TO_PAY` responses call the same idempotent service after response persistence and no longer cause immediate reassessment. Live verification returned creation, scheduled-check, and cancellation audit events.

## 9. Phase 18 merchant profiles

Profiles are immutable and monotonically versioned per merchant. They contain objective, value/retention/contact/cost/fatigue/risk weights, escalation preference, allowlists, minimum NERV, discount budget, and review budget.

## 10. Profile-aware ranking

`nba-v2-profiled` calculates existing candidate economics and applies profile weights with integer fixed-point arithmetic. Profile ID/version and a full snapshot are saved with every decision. Hard eligibility and policy remain authoritative.

## 11. Profile explainability

Candidate rows retain gross value, costs, penalties, NERV, weighted score, rank, and reason codes. Tests prove two profiles can rank identical predictions differently.

## 12. Phase 19 attribution contract

Categories are `DIRECT_ACTION_ATTRIBUTED`, `RETRY_ATTRIBUTED`, `PTP_ATTRIBUTED`, `NATURAL_RECOVERY`, and `UNKNOWN`. Immutable records contain payment, related object IDs, evidence, strength, rule version, and timestamps.

## 13. Attribution precedence

`attribution-v1` checks promise evidence, retry evidence, direct action evidence, no-prior-execution natural recovery, then unknown. Windows come from immutable `attribution_rule_configs`.

## 14. Attribution idempotence

`(case_id, payment_reference)` is unique. A duplicate returns the original record. A first observation attributes the payment, moves the case to `RECOVERED`, emits state/attribution/completion events, and fulfills a matching promise transactionally.

## 15. Phase 20 feedback

Attributed outcomes materialize immutable feedback from the frozen decision, probabilities, snapshots, selected action, policy result, model/profile versions, NERV, costs, amount, and time to outcome. Unknown attribution is retained but excluded from training with a reason.

## 16. Versioned datasets

The dataset builder accepts feedback records, excludes flagged/non-production rows, performs deterministic chronological 70/15/15 splitting, writes canonical JSONL, calculates SHA-256, and writes a manifest. Existing versions cannot be overwritten.

## 17. Model registry

Candidates store model/feature/dataset versions, algorithm, training timestamp, validation and calibration metrics, artifact URI, and artifact hash. Status derives from immutable events: `CANDIDATE`, `APPROVED`, `ACTIVE`, `RETIRED`, or `REJECTED`.

## 18. Explicit promotion

Allowed transitions are candidate to approved/rejected, approved to active/rejected, and active to retired. Activation retires a prior active model of the same type. Live verification rejected candidate-to-active, then accepted reviewed approval and activation.

## 19. Phase 21 calibration

Calibration supports raw probabilities, Platt/sigmoid, and isotonic regression. Fitting and selection use validation data only; test data remains reserved for final evaluation.

## 20. Calibration reporting

Reports include Brier score, expected calibration error, reliability bins, and optional segment Brier scores. The chosen method and selection split are explicit.

The governed candidates compared all three methods. Outcome gradient boosting selected sigmoid (`Brier 0.175082`, `ECE 0.022627`); natural-recovery gradient boosting selected raw probabilities (`Brier 0.174361`, `ECE 0.010293`). Artifacts are stored under `models/candidates/phase21` and are not activated by training.

## 21. Phase 22 priority

`portfolio-priority-v1` loads each actionable case’s newest top-ranked decision. Priority is `expected_NERV × urgency_BPS × recoverability_BPS / 10,000²`. Urgency rises near deadline and recoverability uses selected-action probability.

## 22. Priority durability

Each item stores inputs, score, rank, formula explanation, algorithm version, and run ID. Ties break by NERV and case ID. The merchant priority-run endpoint persists and returns the queue.

## 23. Phase 23 allocation

Allocation enforces spend, contact, retry, discount, and human-review limits. Every candidate records consumption, inclusion, rank, and a precise exclusion reason. Limits cannot be exceeded.

## 24. FCFS and greedy

`budget-fcfs-v1` orders by arrival. `budget-greedy-v1` orders by expected NERV per cost, then priority. Both use identical constraints and persistence for controlled comparison.

## 25. Phase 24 protocol

One command generates 5,000 cases for seeds `101, 202, 303, 404, 505`, evaluates no-recovery, fixed-retry, rules, contextual-retry, and learned NBA with the governed Phase 21 candidates on each test split, and reports aggregate plus checkout/subscription results. Outputs include `summary.json`, per-seed results, and `chart_data.jsonl`.

## 26. Results

Learned NBA averaged `53,883,207` minor units net (95% seed-variation interval `53,140,024`–`54,626,390`) and `42.37%` recovery. Fixed retry averaged `43,101,755` and `32.32%`; rules `46,750,348` and `36.05%`; contextual retry `33,001,248` and `26.85%`; natural/no-recovery `27,262,614` and `22.08%`. NBA mean intervention cost was `139,317`.

## 27. Verification

All five populations contained 5,000 cases, produced distinct test hashes, and passed hidden-feature checks. All Go packages pass; 32 Python tests pass. Docker builds, migration 7 applies, services are healthy, and live promise, profile, priority, allocation, attribution, feedback, and model-promotion paths were exercised.

## 28. Limitations and reproduction

Synthetic potential outcomes are not production causal evidence. Confidence intervals describe seed variation only. Provider communication remains Test Mode/local capture unless configured. Reproduce with `go test ./...`, `python -m unittest discover -s tests -v`, and `python -m evaluation.full_evaluation --dataset-size 5000 --seeds 101 202 303 404 505 --output-dir evaluation/results/phase24`.
