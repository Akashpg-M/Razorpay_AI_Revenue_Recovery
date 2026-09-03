# Judge demo guide

## Preflight

From Git Bash, start the stack and validate the current Razorpay Test Mode tunnel:

```bash
docker compose up -d --build
sh scripts/demo-preflight.sh
```

The preflight verifies schema `phase_55`, all three application surfaces, Razorpay Test Mode authentication, configured webhook HMAC, the public tunnel, and the containerized live credential check. It never prints secrets.

## Nine-step walkthrough

1. Open `http://localhost:3000/demo` and show the provider posture.
2. Create **Checkout abandonment** to demonstrate autonomous bounded execution.
3. Open the case and follow Detect → Diagnose → Decide → Bound → Authorize → Execute → Observe → Attribute → Learn/Audit.
4. Explain the persisted observable-only context, eligibility exclusions, probability uplift, NERV waterfall, gate, and policy.
5. Create **High-value checkout**, open Operations, and demonstrate human approval plus fresh reauthorization.
6. Open the Razorpay Test Payment link. This is Test Mode and uses no real money.
7. Complete payment, wait for the signed webhook, and show provider evidence plus attribution lineage. A created link is never presented as recovered revenue.
8. Open Evaluation to show frozen multi-seed strategy/ablation evidence separately from live operational distributions.
9. Open Resilience and run duplicate webhook, decision-service timeout, stale decision, and response-loss scenarios.

## Promise-to-Pay proof

Create **Active Promise-to-Pay**, open its case, and inspect the durable check and transition rows. The case page exposes development-only Fulfilled and Broken controls. Broken performs the real reliability update, state reassessment, and a new NBA decision. These buttons simulate the customer promise outcome only; they are not Razorpay payment observations.

## Restore the demo baseline

The reset deletes only dynamically created frontend demo cases (`checkout_*` and `pay_demo_*`) from operational tables, then reapplies canonical fixtures. It does not delete evaluation artifacts or provider-side Razorpay Test Mode objects.

```bash
REQUIRE_DEMO_RESET=YES sh scripts/demo-reset.sh
```
