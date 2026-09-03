# RecoverOS Dashboard Presenter Scripts and Recovery Flows

**Format:** six independent presentations of approximately five minutes each, followed by two end-to-end recovery explanations.  
**Current UI:** Phase 55.  
**Evidence rule:** operational values come from the current PostgreSQL database and can include demo/Test Mode records; evaluation values are frozen synthetic held-out results; Razorpay is Test Mode only.

The scripts use the top navigation and ordinary scrolling. They do not depend on the quick-navigation section of the Demo page.

## 1. Command Center / portfolio control plane — five-minute dialogue

**Route:** `/`

### 0:00–0:40 — Product purpose

**Show:** Page hero and the two hero buttons.

**Say:**

“This is the RecoverOS Command Center. RecoverOS starts where ordinary payment tooling usually stops. A failed payment or abandoned checkout tells us revenue is at risk, but it does not tell us that retrying immediately is the right answer. The customer may recover naturally, need another payment method, have an active promise to pay, or be protected by contact limits. RecoverOS decides whether intervention is worthwhile, selects the best permitted action, executes it durably, and only counts recovery when evidence supports attribution.”

### 0:40–1:30 — Operational outcome cards

**Show:** The six cards under **LIVE / OPERATIONAL — Persisted runtime truth**.

**Say:**

“These cards are the current persisted runtime view. Revenue at risk is the total value represented by recovery cases. Recovered means an outcome was actually observed; executing an action does not increase this number. Agent-attributed recovery has action lineage, while Natural recovery represents payment without a qualifying intervention. Awaiting review shows value that policy has deliberately withheld from autonomous execution, and Scheduled shows durable work waiting for or being handled by the worker.

These values are operational database evidence, but this environment can contain controlled demo and Razorpay Test Mode cases. I am not presenting them as production merchant revenue.”

### 1:30–2:20 — Provider and evaluation scopes

**Show:** **RAZORPAY TEST MODE** card, then **SYNTHETIC EVALUATION** card.

**Say:**

“The two cards below deliberately separate two types of proof. The Razorpay card reports whether the selected provider is connected and authenticated in Test Mode and whether signed webhook handling is configured. Test Mode proves the external integration without moving real money.

The Synthetic Evaluation card is different. Its recovery rate and net value come from frozen held-out simulated populations. Those results compare decision strategies under controlled potential outcomes; they are not mixed into the operational recovery totals.”

### 2:20–3:05 — Attribution over time

**Show:** **Cumulative Recovery Attribution**.

**Say:**

“This cumulative curve moves only when payment evidence is attributed. RecoverOS makes a strict distinction between three moments: an action was scheduled, a provider effect such as a Payment Link was created, and a payment was actually observed and linked to the recovery case. Only the third can contribute recovered revenue.

This protects the system from giving itself credit for a link that nobody paid, or for money that would have recovered naturally.”

### 3:05–4:10 — Recovery portfolio

**Show:** **Recovery Portfolio — Recent cases and next actions**. Point across each column.

**Say:**

“The portfolio turns the headline numbers into accountable cases. Each row shows the case, amount at risk, leak type, selected or last action, expected NERV, policy result, and current state. NERV means Net Expected Recovery Value: expected incremental recovery value after action cost, fatigue, and risk.

This state is not cosmetic. Cases move through detection, diagnosis, decision, policy review, scheduling, execution, waiting for outcome, reassessment, and terminal states. Opening Inspect takes us from a metric to the exact probabilities, controls, execution evidence, attribution, and audit events that produced it.”

### 4:10–5:00 — Command Center conclusion

**Show:** Return attention to **Bounded & explainable**, then the operational cards.

**Say:**

“The Command Center therefore answers four executive questions: how much value is exposed, how much was actually recovered, how much recovery has strong action attribution, and where human or worker attention is required.

The central product principle is simple: AI estimates what is likely to work; deterministic systems decide what is allowed to happen. That is why the dashboard can show recovery and control in the same view—revenue outcome, natural recovery, human escalation, durable scheduling, and evidence-backed attribution rather than a raw retry count.”

## 2. Recoveries and case evidence — five-minute dialogue

**Route:** `/recoveries`, then open one case with **Inspect →**

### 0:00–0:40 — Accountable case list

**Show:** Recoveries table.

**Say:**

“The Recoveries page is the case registry. Every revenue leak becomes one accountable RecoveryCase instead of a loose collection of retries and messages. The table shows the merchant, amount and leak type, selected action and expected NERV, policy state, current lifecycle state, and attribution status.

I can start from any row and trace what the system knew, what it predicted, what it allowed, what it executed, and whether that execution recovered revenue.”

### 0:40–1:15 — Nine-stage pipeline and detection

**Do:** Open a recent case. Show the hero, nine-stage pipeline, and **DETECT** card.

**Say:**

“Inside the case, the nine-stage pipeline is the narrative: Detect, Diagnose, Decide, Bound, Authorize, Execute, Observe, Attribute, and Learn or Audit. Completed stages are based on persisted evidence.

The Detect section shows the source reference, merchant, customer reference, leak type, deadline, recovered amount, attribution state, and case version. The raw normalized snapshot is available when deeper inspection is needed.”

### 1:15–1:55 — Diagnosis and observable evidence

**Show:** **DIAGNOSE · DETERMINISTIC**.

**Say:**

“Diagnosis maps normalized payment or checkout evidence into a failure category, recoverability, confidence, and evidence list. It is deterministic; a generative model does not invent this label.

The visible customer state includes recent failures, previous recovery attempts, fatigue, and promise reliability. Simulator-only hidden traits are prohibited from this operational decision context, so the optimizer sees only information that would actually be observable.”

### 1:55–3:00 — Decision detail

**Show:** **DECIDE**, the four headline values, probability bars, candidate table, and NERV waterfall.

**Say:**

“The statistical layer estimates the probability of natural recovery under WAIT and recovery probability for each eligible action. Incremental uplift is action probability minus natural probability. That difference matters because RecoverOS should not claim credit for recovery likely to happen without intervention.

The candidate table ranks multiple possible actions. It shows action probability, natural baseline, uplift, gross incremental value, total costs, and NERV. The waterfall makes the economic calculation explicit: incremental value minus action cost minus fatigue and risk equals NERV.

The highlighted row is the Next-Best-Action for the merchant objective. It is not necessarily the highest raw probability; it is the strongest permitted objective-adjusted value.”

### 3:00–3:45 — Bounds and authority

**Show:** **BOUND**, then **AUTHORIZE**.

**Say:**

“Before execution, deterministic controls take over. Eligibility shows both allowed and excluded candidates with reasons. The Economic Gate compares selected NERV with the merchant threshold. Policy then applies opt-out, quiet hours, contact and retry caps, active promises, payment-method state, deadlines, high-value escalation, and terminal-state protection.

Authorize shows whether the case received autonomous policy authority or a human decision. When a person approves, the page also shows the fresh reauthorization result and version evidence.”

### 3:45–4:30 — Execution, observation, attribution

**Show:** **EXECUTE**, **OBSERVE**, and **ATTRIBUTE**.

**Say:**

“Execution exposes the durable schedule, attempts, stable idempotency key, and provider reference. Observation shows signed provider webhook evidence. Attribution then traces a payment through reference, execution, action, decision, and RecoveryCase.

This is why Payment Link created, payment observed, and revenue attributed are three different statements. The case becomes RECOVERED only after the outcome is resolved and attributed.”

### 4:30–5:00 — Replay

**Show:** **LEARN / AUDIT · IMMUTABLE HISTORY**. Expand one decision or recovery event.

**Say:**

“Finally, the replay is the explainability record. Ordered immutable events sit beside the full decision, policy, execution, provider, attribution, feedback, and version provenance. A reviewer does not have to trust a summary metric; they can inspect the evidence chain that created it. That makes each recovery reproducible, contestable, and auditable.”

## 3. Operations / human-in-the-loop control plane — five-minute dialogue

**Route:** `/operations`

### 0:00–0:45 — Why Operations exists

**Show:** Page hero and four KPI cards.

**Say:**

“Operations is the human-in-the-loop control plane. RecoverOS is allowed to handle ordinary safe cases autonomously, but high-value or low-confidence decisions stop here for judgment.

The top cards show pending reviews, value awaiting review, approved versus rejected decisions, and stale approvals that were blocked. This is bounded escalation: people focus on exceptional economic or risk cases rather than manually approving every recovery attempt.”

### 0:45–1:35 — Queue prioritization

**Show:** Priority filter and the first reviewable card.

**Say:**

“The queue can be filtered by critical, high, or normal priority. A review card gives the operator the minimum decision evidence required: amount at risk, safe merchant and customer references, recommended action, expected NERV, action and natural recovery probabilities, incremental uplift, diagnosis confidence, merchant objective, deadline, and escalation reasons.

This is not a black-box approve button. The reviewer can see both the predicted benefit and why the deterministic policy refused autonomous authority. Priority combines business urgency with the recovery deadline and expected value, so the queue helps people focus without changing the underlying policy decision.”

### 1:35–2:25 — Review options

**Show:** **Approve safely**, **Defer**, **Reject**, **Stop case**, **Full evidence →**.

**Say:**

“The operator has four explicit choices. Approve safely accepts the recommendation subject to reauthorization. Defer records a future review time. Reject declines this recommendation without pretending the case disappeared. Stop case moves the recovery into a terminal stopped state. Full evidence opens the complete case before deciding.

Every review requires an operator identity, reason code, notes, and idempotency key, and the review becomes immutable evidence.”

### 2:25–3:25 — Fresh reauthorization

**Do:** If using a newly created high-value demo case, click **Approve safely**, accept `demo-operator`, enter a short note, and confirm.

**Say:**

“I’ll approve this current high-value recommendation. Human approval is an expression of intent, not permission to bypass newer facts.

RecoverOS locks and reloads the case, compares the expected and current case versions, confirms the merchant-policy version, checks that the deadline is open, retains the Economic Gate result, and runs current policy again. Only then does the result show Fresh authorization APPROVED and create a durable schedule. If the case changed while I was reviewing it, the system would record a stale approval and would not execute the old decision.”

### 3:25–4:30 — Stopping rules

**Show:** **DETERMINISTIC CONTROLS — Stopping rules**. Move through each row.

**Say:**

“The lower section shows the controls that bound both autonomous and human-reviewed cases. An active Promise-to-Pay suppresses competing contact and retries. Quiet hours delay contact. Daily and weekly contact caps prevent fatigue. Retry caps prevent repeated collection attempts. Already recovered and expired-window checks stop stale work. Merchant STOP is terminal. Stale decision and stale approval checks prevent old authority from reaching execution.

These machine-readable reason codes appear in policy and replay, so a denied action is explainable rather than silently missing. The operator cannot erase these limits by pressing Approve: the action must still be permitted by the current customer state, merchant configuration, and recovery window.”

### 4:30–5:00 — Operations conclusion

**Show:** Return to the queue KPI cards.

**Say:**

“Operations demonstrates compliant escalation without turning RecoverOS into a manual workflow. Safe work remains autonomous; exceptional work receives accountable human judgment; and even an approval must still pass fresh deterministic checks. The result is faster recovery with authority that remains current, bounded, and auditable.

After approval, execution still happens asynchronously through the durable worker. The browser does not call Razorpay directly, and closing this page cannot lose the scheduled action. Operations supplies authority; the worker remains responsible for safe execution and the case replay remains responsible for proof.”

## 4. Evaluation — five-minute dialogue

**Route:** `/evaluation`

### 0:00–0:45 — Evidence scope and methodology

**Show:** **SYNTHETIC HELD-OUT EVALUATION** and the evidence strip.

**Say:**

“This page is explicitly SYNTHETIC HELD-OUT EVALUATION. These are not production revenue claims. The purpose is to compare strategies on controlled, reproducible populations before relying on scarce real recovery outcomes.

The frozen run uses five seeds, 5,000 generated cases per seed, 25,000 total cases, and 3,750 held-out cases evaluated per strategy. Train, validation, and held-out splits are 70, 15, and 15 percent.”

### 0:45–1:30 — Reproducibility

**Show:** **REPRODUCIBLE METHODOLOGY**, seed hashes, and model hashes.

**Say:**

“The evaluation stores a version, individual held-out dataset hashes for each seed, and hashes for the outcome and natural-recovery model artifacts. The same seed produces the same population and potential outcomes. Integrity checks also verify that simulator-only hidden customer characteristics never enter model input.

The simulator covers subscription failures and checkout abandonment across merchant and customer segments. Hidden traits affect simulated outcomes, while models and strategies receive only observable payment history, failure, tenure, method, contact, and merchant context. This makes the comparison repeatable rather than a hand-selected collection of successful cases.”

### 1:30–2:35 — Strategy comparison

**Show:** Strategy graph and comparison table.

**Say:**

“Five strategies see the same held-out populations. No Recovery always waits and reaches 22.1 percent recovery with about ₹2.73 lakh mean net recovered value. Contextual Retry reaches 26.9 percent and about ₹3.30 lakh. The Fixed Strategy reaches 32.3 percent and about ₹4.31 lakh. Rules reaches 36.1 percent and about ₹4.68 lakh. RecoverOS Full NBA reaches 42.4 percent and about ₹5.39 lakh.

Net value is gross recovered value minus intervention cost. The comparison therefore rewards recovery but does not hide the cost of producing it. The underlying reports also retain contact and attempt behavior, allowing a strategy to be judged on efficiency and customer impact instead of recovery rate alone.”

### 2:35–3:25 — Why NERV matters

**Show:** **WHY NERV MATTERS** and **NERV-Greedy vs FCFS**.

**Say:**

“The left side summarizes the decision logic: natural recovery probability, action recovery probability, uplift, expected incremental value, costs plus fatigue and risk, then NERV.

The portfolio comparison applies identical simulated spend, contact, and retry capacities. First-come-first-served produces about ₹5.07 lakh aggregate expected NERV, while NERV-Greedy produces about ₹8.56 lakh—a 68.7 percent gain in expected NERV. This is a synthetic allocation result, not observed production revenue.”

### 3:25–4:15 — Ablations

**Show:** **ABLATION EVIDENCE**.

**Say:**

“Ablations test what changes when individual capabilities are removed: customer context, merchant context, natural recovery, fatigue cost, non-retry actions, Promise-to-Pay, Economic Gate, policy-aware optimization, or calibration.

The results are intentionally honest. Not every removal makes every metric worse in this simulator. For example, removing some controls can increase raw simulated recovery while increasing contact or weakening safety. The ablation view helps us distinguish revenue optimization from the controls required for a responsible financial workflow.”

### 4:15–5:00 — Operational distributions and conclusion

**Show:** **LIVE ROOT-CAUSE DISTRIBUTION**, **LIVE ACTION DISTRIBUTION**, and the scope note.

**Say:**

“The final two charts are deliberately different: they come from current operational and Test Mode cases, showing which root causes and selected actions are actually present in this database. They are not blended with the strategy results above.

So this page proves three things: the evaluation is reproducible, Full NBA is measured against simpler baselines on identical held-out populations, and portfolio value can be optimized under constraints. It does not claim production causal lift; that would require production experimentation and causal measurement.

The safe conclusion is relative and scoped: within this frozen simulator, the full decision system outperforms the listed baselines on the headline recovery and net-value measures. Operational attribution remains a separate evidence source.”

## 5. Reliability / Resilience Lab — five-minute dialogue

**Route:** `/resilience`

### 0:00–0:40 — Scope of the lab

**Show:** Page hero and **DETERMINISTIC FAULT SIMULATION** label.

**Say:**

“The Resilience Lab breaks important workflow assumptions on purpose. These are deterministic Go domain and worker simulations whose evidence is persisted. They exercise the actual authorization and worker boundaries, but they are not presented as a real Razorpay outage or production chaos test.

Each button names the invariant it protects, and the result panel reports worker attempts, provider calls, modeled external effects, duplicates blocked, reconciliation activity, and the final suppression or completion state.”

### 0:40–1:30 — Webhook integrity

**Do:** Click **Duplicate Webhook ×10**, wait for PASS, then point to the metrics.

**Say:**

“Here I deliver the same modeled provider event ten times. The result shows ten deliveries, nine duplicates blocked, and one bounded business effect. The invariant is not that webhooks arrive once; providers use at-least-once delivery. The invariant is that repeating the same provider event cannot repeat recovery mutation.

The Invalid Webhook Signature scenario protects another subtle boundary: authentication happens before an event ID is reserved.”

### 1:30–2:15 — Fail-closed prediction

**Do:** Click **Decision Service Timeout** and wait for PASS.

**Say:**

“A decision-service timeout produces no external effect. RecoverOS does not guess an action or fall back to a blind retry when prediction is unavailable. It fails closed before scheduling.

That is important because the model is advisory. Eligibility, economics, policy, versions, and authority remain deterministic, and an unavailable estimate cannot create permission.”

### 2:15–3:05 — Worker durability

**Do:** Click **Worker Crash**, then optionally **Expired Lease**.

**Say:**

“The worker scenarios demonstrate durable scheduling. Work is persisted before execution, claimed with an owner and lease, and can be reclaimed after a crashed worker’s lease expires. Stable idempotency and reconciliation prevent the modeled external effect from being repeated.

The accurate guarantee is at-least-once workflow processing with idempotent or reconciled external side effects. We do not claim exactly-once execution across PostgreSQL and an external provider.

Related cards cover duplicate scheduled jobs, provider timeout, and success with a lost response. Together they distinguish retrying workflow processing from repeating a provider-side effect.”

### 3:05–3:50 — Stale and reordered state

**Do:** Click **Stale Decision**, then point at **Customer Pays First** and **Out-of-Order Event** cards without running all of them.

**Say:**

“A stale decision is suppressed when the case version changes before execution. Customer Pays First checks current recovery state and suppresses the pending action. Out-of-order events remain bounded by provider-event uniqueness and terminal-state guards.

These cases prove that historical AI output cannot override newer payment, policy, or lifecycle facts.”

### 3:50–4:35 — Engineering lessons and limitation

**Show:** **WHAT BROKE · ROOT CAUSE · FIX · PROOF** stories.

**Say:**

“The strongest engineering lesson was webhook ordering. If an invalid-signature request reserves the event ID first, it can poison deduplication and block the later genuine event. RecoverOS now verifies HMAC before reserving that ID.

Provider response loss is more nuanced. Reconciliation can reuse a Razorpay Payment Link when its reference was persisted. If provider success occurs before that reference reaches PostgreSQL, the current Razorpay adapter cannot always rediscover it. That limitation remains documented rather than hidden behind the simulated PASS.”

### 4:35–5:00 — Reliability conclusion

**Show:** A final PASS result and its persisted run ID.

**Say:**

“The lab demonstrates bounded failure: duplicates do not multiply effects, model failure does not invent authority, crashed work can be reclaimed, stale decisions do not execute, and paid cases suppress pending actions. Each run has persisted evidence and a run ID, turning reliability claims into repeatable demonstrations.

The point is not that failures disappear. The point is that failure has an explicit state, a bounded next step, and evidence an operator can inspect instead of an ambiguous retry loop.”

## 6. Observability — five-minute dialogue

**Route:** `/observability`

### 0:00–0:45 — Runtime posture

**Show:** The six health cells.

**Say:**

“Observability answers whether RecoverOS is safe to demonstrate and operate right now. The first row shows backend, decision service, PostgreSQL, Redis, worker, and schema status. Phase 55 confirms that the application is running against the expected database schema.

PostgreSQL and the decision service are required for backend readiness. Redis is shown explicitly, but it is non-authoritative in the current system. The worker has its own readiness signal, so API availability and execution capacity are not incorrectly treated as the same condition.”

### 0:45–1:40 — Durable queue and execution

**Show:** Queue and execution cards.

**Say:**

“Queue pending and running come from durable PostgreSQL schedule state, not browser memory. Oldest lag shows how late the oldest due action is. Failed work remains visible instead of disappearing.

Execution metrics separate success, failure, retrying, and timeout. This helps an operator distinguish a healthy empty queue from a worker that is stuck or repeatedly failing. The worker itself exposes separate readiness because the API can be healthy while execution capacity is unavailable.”

### 1:40–2:30 — Webhooks and recovery

**Show:** Webhook and recovery cards.

**Say:**

“Webhook metrics show verified deliveries, processed outcomes, failures, and the most recent receipt. Invalid signatures never enter this count because verification occurs before persistence.

Recovery metrics separate active and recovered cases, with escalated cases and overdue active promises visible. Together, these cards answer whether revenue cases are moving, waiting for people, waiting for outcomes, or accumulating behind a failure.

A failed webhook remains visible for investigation; a duplicate verified webhook does not repeat the business mutation. Recovered counts still come from case and attribution state rather than from a raw webhook-success counter.”

### 2:30–3:30 — Razorpay provider status

**Show:** **PROVIDER STATUS** card and each READY/MISSING row.

**Say:**

“The provider panel shows the selected provider and mode. In the intended demo it should read RAZORPAY TEST MODE. Credentials, Reachable, and Authenticated describe outbound API readiness. Webhook signature confirms that the backend has a webhook secret. Public webhook URL confirms that a URL is configured.

Those last two are configuration evidence, not proof that the temporary tunnel is still running or that the Razorpay Dashboard points to it. That is why the preflight separately calls backend readiness through the public tunnel.”

### 3:30–4:20 — Alerts

**Show:** **Active alerts**.

**Say:**

“Alerts turn metrics into operator attention. Queue Lag High appears when the oldest due action is more than five minutes late. Execution Failures appears when failed schedules or executions require inspection. Promise Checks Overdue appears when an active promise has passed its due time.

When no threshold is breached, the page says No thresholds breached rather than hiding the alert surface. From here, an operator can use Recoveries for case evidence or Operations for human-review work; Observability identifies the condition without pretending to replace investigation.”

### 4:20–5:00 — Observability conclusion

**Show:** Sweep from health through provider status and alerts.

**Say:**

“This page connects business operation with technical readiness: required services, schema, worker capacity, durable queue, execution outcomes, signed webhook processing, recovery state, Promise-to-Pay checks, provider posture, and alerts.

It does not infer recovery from service health. A green provider status means the integration is ready; recovered revenue still requires a correlated payment observation and attribution. That separation keeps operational confidence and business outcome honest.

In short, this is a runtime proof page, not a vanity uptime panel. It connects readiness, backlog, execution, provider delivery, business state, and alert thresholds while preserving the distinction between a healthy system and a successful recovery.”

## 7. ₹2,499 autonomous checkout-abandonment flow

### What to run

1. Open `/demo`.
2. Find **Checkout abandonment** with **₹2,499 payment-friction abandonment**.
3. Click **Create & run decision**.
4. Wait for **GUIDED LIVE JOURNEY**, then click **Open full case →**.

### What happens and how to explain it

| Stage | Current behavior | Presenter explanation |
|---|---|---|
| Detect | The demo submits `CHECKOUT_STARTED`, then `CHECKOUT_ABANDONED`. Checkout state supplies merchant, customer, amount, stage, and deadline. A unique source reference prevents duplicate cases. | “RecoverOS turns checkout abandonment into one stateful ₹2,499 revenue-risk case.” |
| Diagnose | Payment friction is normalized and deterministically mapped to recoverability/confidence. | “The cause comes from normalized evidence, not generated text.” |
| Decide | Observable context is assembled. Eligibility runs before model scoring. The natural model scores WAIT; the outcome model scores eligible executable actions. | “The model compares what may happen naturally with what may happen under each valid action.” |
| Optimize | Go calculates uplift, gross incremental value, costs/penalties, NERV, and merchant-objective ranking. | “The selected action must add incremental value after cost, fatigue, and risk.” |
| Bound | The Economic Gate checks NERV against the merchant threshold. Policy checks all deterministic protections. | “A high probability alone cannot authorize execution.” |
| Authorize | ₹2,499 is below the seeded ₹5,000 high-value threshold, so a safe allowed result can receive `APPROVE` without entering Operations. | “This is bounded autonomy: policy—not the model—grants authority.” |
| Schedule | For an approved non-WAIT action, the decision, action, schedule, and case transitions commit atomically in PostgreSQL. | “The browser does not perform the provider effect; it creates durable work.” |
| Execute | The worker claims the schedule, rechecks current case version/gate/policy, then runs the selected executor. The exact action remains data-driven and should be read from the highlighted candidate row. | “The worker executes only fresh authority.” |
| Outcome | If the current selection is `SEND_PAYMENT_LINK`, Razorpay Test Mode creates the link and the case waits. If another executable action wins, its current registered executor handles it. | “Execution success is not recovery; the case still needs outcome evidence.” |
| Attribute | A paid, signed, correlated Payment Link can become direct-action attribution and `RECOVERED`. An unpaid link remains `WAITING_OUTCOME`; a timeout can trigger reassessment. | “RecoverOS only counts what the evidence supports.” |

### What this case proves

- Real checkout-risk detection and persisted state.
- Natural recovery versus action-conditioned prediction.
- Next-Best-Action selected by incremental economics.
- Economic and policy controls before authority.
- Autonomous scheduling below the high-value threshold.
- Durable worker execution rather than frontend-triggered side effects.
- No false recovery claim when a Payment Link is merely created.

## 8. ₹8,999 high-value human-in-the-loop flow

The current UI scenario is **₹8,999 High-value checkout**, not an exact ₹8,000 case. Use the implemented ₹8,999 scenario so every displayed value and threshold matches the code.

### What to run

1. Open `/demo`.
2. Find **₹8,999 High-value checkout**.
3. Click **Create & run decision**.
4. Open the generated case and inspect Diagnose, Decide, and Bound.
5. Use the top navigation to open `/operations`.
6. Approve the newly created current-deadline ₹8,999 card with **Approve safely**.
7. Return with browser Back, reload, and inspect Authorization and Execution.
8. Open **Razorpay Test Payment**, complete a Test Mode success, return, and reload until `RECOVERED`.
9. Inspect Observe, Attribution, and Learn/Audit.

### What happens and how to explain it

| Stage | Current behavior | Presenter explanation |
|---|---|---|
| Detect | The same checkout-start/abandonment path creates a ₹8,999 case with a current recovery deadline. | “The amount changes authority requirements, not the integrity of detection.” |
| Diagnose | Payment friction and confidence are persisted in the observable decision context. | “The diagnosis is explainable and frozen with the decision.” |
| Decide | The models produce natural and action-conditioned probabilities; the optimizer displays all candidate uplift, costs, NERV, and ranking. The seeded scenario is designed for `SEND_PAYMENT_LINK` to win, but the persisted selected row is authoritative. | “AI estimates the most promising eligible action.” |
| Economic Gate | Positive/above-threshold NERV produces `ALLOW`. | “Economics says the intervention is worthwhile, but that is not yet permission.” |
| Policy | ₹8,999 is above the seeded merchant high-value threshold of ₹5,000, producing `ESCALATE` and no immediate schedule. | “Deterministic policy withholds autonomous execution because the value is high.” |
| Human review | Operations displays amount, probabilities, uplift, NERV, confidence, objective, deadline, and escalation reason. | “The reviewer sees decision evidence rather than a blind approval request.” |
| Fresh reauthorization | Approval reloads and locks current state, checks expected case version, merchant-policy version, deadline, stored gate, and current policy. A stale approval records evidence but does not schedule. | “Human approval cannot override newer facts.” |
| Durable schedule | A successful reauthorization records the immutable review and refreshed policy evidence, then moves through action pending and policy review to scheduled in one transaction. | “The approved intent becomes durable, versioned work.” |
| Worker execution | The worker claims with a lease, checks schedule-era versus current state again, confirms the human marker, and creates a Razorpay Test Mode Payment Link containing stable correlation metadata. | “Authority is rechecked immediately before the external effect.” |
| Wait for outcome | The link ID is stored as provider reference; execution is recorded; the case waits. | “Payment Link created is not payment observed and not revenue attributed.” |
| Observe | Razorpay sends `payment_link.paid` through the public tunnel. The backend verifies exact-body HMAC before reserving the event ID and suppresses valid duplicates. | “Only authenticated provider evidence can mutate recovery state.” |
| Attribute | The link resolves to execution, action, decision, and case. Exact matching creates `DIRECT_ACTION_ATTRIBUTED` with `STRONG` evidence. | “The payment is now connected to the intervention.” |
| Recover and learn | Attribution, recovered amount, `RECOVERED`, feedback, and recovery events commit together. Dashboard/replay reads the resulting PostgreSQL evidence. | “Only now is ₹8,999 counted as recovered.” |

### What this case proves

- Full nine-stage workflow on one case.
- AI/statistical prediction separated from deterministic authorization.
- Economic Gate plus high-value human escalation.
- Fresh reauthorization and stale-authority protection.
- Durable worker execution with Razorpay Test Mode.
- Signed webhook verification and duplicate protection.
- Direct-action attribution rather than generic payment success.
- Recovered revenue, feedback, and immutable replay.

## 9. Short comparison to use after both cases

| ₹2,499 autonomous case | ₹8,999 human-reviewed case |
|---|---|
| Same detection and diagnosis discipline | Same detection and diagnosis discipline |
| Same natural/action probability and NERV calculation | Same natural/action probability and NERV calculation |
| Economic Gate and policy can authorize a safe action | Economic Gate may allow value, but high-value policy escalates authority |
| Approved non-WAIT work is scheduled automatically | No schedule exists until human approval passes fresh reauthorization |
| Worker still rechecks current state before execution | Worker rechecks current state plus durable human authority |
| Outcome still requires observation and attribution | Outcome still requires signed observation and strong attribution |

**Closing line:**

“The intelligence is consistent across both cases; the authority is proportional to risk. RecoverOS can act quickly when it is safe, wait or stop when intervention is inappropriate, and require accountable human judgment when the stakes are higher.”
