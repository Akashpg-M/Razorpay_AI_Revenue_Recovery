# Phases 25–30 implementation and verification

1. **Phase 24 cleanup.** The final strategy is `full_nba_agent_v1`; historical intermediate files remain untouched where they represent earlier work.
2. **Case accounting.** The corrected manifest reports 25,000 generated cases and 3,750 held-out cases evaluated per strategy, derived from generator split reports.
3. **Budget comparison.** FCFS and greedy NERV-efficiency allocations use the same frozen spend, contact, and retry budgets. The persisted expected NERV gain is 34,876,675 minor units.
4. **Attribution precedence.** `attribution-v2` orders exact provider reference, promise, retry, direct action, natural, and unknown evidence. Overlap tests freeze that order.
5. **Statistical wording.** Reported intervals describe variation across deterministic simulation seeds, not production-population confidence or causal significance.
6. **Phase 25 design.** Ten configurations—including the full agent and nine single-family removals—run on identical held-out case IDs and potential outcomes.
7. **Ablation outputs.** `ablation_manifest.json`, JSON/CSV summaries, per-seed records, per-case paired deltas, and chart data are persisted under `evaluation/results/phase25`.
8. **Ablation result.** Removing non-retry actions caused the largest mean lost net recovery. Some removals were neutral, while customer/merchant context removals improved realized synthetic means; this is retained as a model-improvement signal, not hidden.
9. **Phase 26 safety evaluation.** Twenty-two restricted cases execute through `orchestrator.Worker.RunOnce`, including customer, payment, terminal-state, timing, channel, economic, and escalation controls.
10. **Unsafe action rate.** Covered restricted fixtures produced 0 unsafe external calls and 0 external side effects; this is a test-harness measurement, not a universal production claim.
11. **Phase 27 reliability matrix.** Fourteen required fault classes cover duplicate delivery, retries, crashes, timeouts, ordering, leases, concurrency, stale work, and Redis unavailability.
12. **Duplicate side-effect rate.** The local deterministic fault adapters produced 0 duplicate external effects across the matrix.
13. **Provider ambiguity.** When a provider effect succeeds but its response is lost, retry reconciles the stable provider key before execution: one provider call and one external effect.
14. **Phase 28 scenarios.** Eleven visible scenarios include duplicate webhook, worker crash, Razorpay timeout, invalid model output, duplicate job, out-of-order event, ambiguous provider success, lease expiry, Redis outage, stale NBA decision, and payment before schedule.
15. **Resilience routes/APIs.** `/resilience` calls `POST /api/v1/resilience/scenarios/:scenario/run`; `GET /api/v1/resilience/runs/:id` reloads the append-only PostgreSQL record. Both API routes are disabled outside development/demo/test.
16. **Phase 29 data sources.** Live/test KPIs come from PostgreSQL recovery cases and attributions. Synthetic comparisons come from mounted immutable evaluation artifacts.
17. **Baseline comparison.** The dashboard renders full NBA and frozen baseline mean net-recovery values from Phase 24 chart data.
18. **Attribution breakdown.** Operational natural recovery is displayed separately from direct, retry, and promise agent-attributed recovery.
19. **Portfolio result.** The same-budget FCFS-versus-greedy comparison is displayed from `budget_comparison.json`, not hardcoded UI data.
20. **Phase 30 replay.** The replay API returns case/merchant, immutable events, all decisions/candidates, gates, policies, actions, schedules, executions, promises, attributions, and version provenance.
21. **Verified replay.** Persisted case `23d419d7-2756-4257-b202-9c104c5ee26a` returned HTTP 200 with 26 events, 3 decisions, 5 candidates, 1 execution, 1 promise, and 1 attribution.
22. **PTP/reassessment visibility.** Promise records and their state-changing audit events appear in the replay alongside repeated decisions and schedules.
23. **Provenance.** Replay includes context, outcome model, natural model, optimizer, merchant-profile, policy, and attribution-rule versions when recorded.
24. **Verification.** Go tests pass across all packages; 36 Python unittests pass after the new tests; ESLint and the Next.js production build pass; migration 8 is clean; all six long-running services are up and all five services with health checks are healthy.
25. **Known limitations.** Simulator outcomes are not production causal estimates. Fault providers are deterministic local adapters. External delivery is at-least-once with idempotency/reconciliation, not universally exactly once.
26. **Failures fixed.** This block fixed stale original-decision version reauthorization, absent provider reconciliation, vague held-out counts, unversioned overlap precedence, and misleading final strategy naming. Earlier real failures remain visible in the Resilience Lab.
27. **Reproduce evaluations.** From `decision-service`, run `python -m evaluation.full_evaluation --dataset-size 5000 --seeds 101 202 303 404 505` and `python -m evaluation.ablations --dataset-size 5000 --seeds 101 202 303 404 505`; from `backend`, run `go run ./cmd/evaluation ../decision-service/evaluation/results`.
28. **Run the demo.** Run `docker compose up -d --build`; open `/`, trigger `/resilience`, then choose any case link or open `/recovery/{case_id}`. The example recovered case above is currently persisted in the development database.
29. **Phase 31+.** Production provider-specific reconciliation lookup, authenticated operator roles, live observability/SLOs, production causal evaluation, and deployment hardening remain future work.
