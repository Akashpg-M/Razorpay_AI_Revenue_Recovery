# Claims and Evidence

| Claim | Evidence | Scope | Limitation |
|---|---|---|---|
| Revenue risk is detected from failed payments and checkout abandonment. | Scenario Lab invokes the real detection adapters; cases and `REVENUE_RISK_DETECTED`/`CASE_CREATED` events are persisted. A signed Cloudflare test event created case `7f3fb56f-61bf-4f2d-a1c8-f0281f0770bf`. | Operational PostgreSQL flow; signed synthetic ingress verification. | The injected event is not a real customer failure. |
| RecoverOS chooses by incremental value, not raw success probability. | Recovery Decision UI shows natural and action-conditioned probabilities, uplift, gross value, cost components, NERV, rank and versions from persisted decision rows. | Runtime decisions and synthetic models. | Predicted probabilities are model estimates. |
| High-value recovery is bounded by human authority. | Case `d4ff7fda-0010-4b8a-acba-46a849000102`, decision `50eeb824-4f8d-4be6-80fc-1acc4948d659`, gate `e72e6ffc-fb4c-450e-9155-65cd0ed6206d`, and approval `bdaf728d-36c9-4d9e-922e-04398281fcfe`. | Verified local stack. | Demo operator authentication is deployment-owned. |
| Razorpay Test Mode execution works. | Schedule `fbcd9b58-6d7d-4729-8ee6-2e7c2e43a513` produced execution/provider reference `plink_TXAB1tY0qLM5Uf` and Test Mode URL through authenticated Razorpay API. | Razorpay Test Mode; no real money. | A person must complete the hosted test checkout. |
| Signed public webhook delivery works. | Cloudflare readiness returned 200; signed event `cloudflare-verification-5ffdb25b-47c9-408c-a294-0d1638e19ca3` passed HMAC validation and created exactly one risk case. | Temporary Cloudflare tunnel and configured secret. | Tunnel hostname expires when the quick tunnel stops. |
| Payment Link success automatically attributes and recovers. | Source/tests trace `payment_link.paid` through persisted link resolution to `recovery_attributions`, feedback and terminal events. Replay exposes all links. | Razorpay Payment Link contract. | The final fresh Test Mode link was not paid automatically because that is an external user action. |
| Duplicate and crash failure modes are bounded. | Persisted Reliability runs `9f258009-063c-4eb3-827a-4856fd3d4e2b` (duplicate webhook) and `c7ece399-050f-4986-bb4e-3806ce78c609` (worker crash) passed with one provider effect. | Deterministic Go worker/domain simulations. | These are simulations, not Razorpay availability guarantees. |
| Batch evaluation measures recovered value against baselines. | Evaluation page reads frozen multi-seed artifacts under `decision-service/evaluation/results/phase24` and ablations under `phase25`. | Synthetic held-out evaluation. | Not production causal evidence. |
| Audit history is immutable and judge-visible. | Recovery Replay joins events, decisions, candidates, gate, policy, reviews, schedules, executions, provider refs, webhooks, attributions and feedback. Database constraints/triggers enforce sequence and append-only behavior. | Persisted operational cases. | Infrastructure administrators still control the database deployment. |

## Final demonstration wording

Use “Razorpay Test Mode,” “live operational case with synthetic input,” and “synthetic held-out evaluation” exactly as labelled in the UI. Do not combine their revenue totals or describe Test Mode money as production revenue.
