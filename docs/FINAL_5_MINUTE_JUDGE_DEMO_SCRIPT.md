# RecoverOS — Final 5-Minute Judge Demo Script

**Target duration:** 5:00  
**Evidence scopes used:** live operational workflow with synthetic demo input; Razorpay Test Mode; synthetic held-out evaluation; deterministic fault simulation  
**Navigation rule:** use the top navigation bar, normal scrolling, browser Back, and direct case links. Do not use the quick-navigation tabs on the Demo page.

## 1. Final 5-minute transcript

### 0:00–0:22 — The problem

“A failed payment is not automatically a retry problem. Some customers recover naturally, some need another payment method, and some should not be contacted at all. Blind retries waste money, increase fatigue, and can violate merchant or customer limits. RecoverOS turns each revenue leak into a bounded recovery decision—and only claims revenue when the payment can be attributed.”

### 0:22–0:42 — Autonomous safe case

“I’ll start with this ₹2,499 checkout abandonment. The signal creates a real persisted case, RecoverOS diagnoses the payment friction, compares eligible actions, applies economics and policy, and—because this case is within the safe boundary—can authorize and schedule the selected action without human delay.”

### 0:42–1:35 — Main ₹8,999 case and Next-Best-Action

“Now this ₹8,999 abandonment is the main journey. Across the case page you can see the nine stages: Detect, Diagnose, Decide, Bound, Authorize, Execute, Observe, Attribute, and Learn or Audit.

The diagnosis is deterministic and based on normalized evidence. In Decide, the statistical models estimate two things: the chance of natural recovery if we wait, and the chance of recovery for each eligible action. Their difference is incremental uplift.

RecoverOS converts that uplift into expected incremental value, subtracts action cost, fatigue, and risk, and produces NERV—Net Expected Recovery Value. That gives us the Next-Best-Action, not simply the action with the highest raw probability. Then the Economic Gate checks whether intervention is worthwhile, and merchant policy checks whether it is allowed.”

### 1:35–2:05 — Human authority

“The gate allows this action, but policy escalates it because the amount is high. This is the key separation: AI estimates what is likely to work; deterministic systems decide what is allowed to happen.

I’m approving the recommendation in Operations. Approval is not a bypass. RecoverOS performs Fresh Reauthorization against the current case version, deadline, economic result, merchant policy, and customer protections before it creates durable work.”

### 2:05–2:35 — Durable execution

“The approval is recorded, the worker claims the scheduled action, rechecks authority immediately before execution, and creates a real Razorpay Test Mode Payment Link. The schedule, attempt, stable idempotency key, and provider reference are persisted. This is at-least-once workflow processing with bounded, idempotent or reconciled effects—not an exactly-once claim.”

### 2:35–3:05 — Test Mode payment

“This hosted page is Razorpay Test Mode, so no real money moves. I’ll complete a successful test payment now. At this moment, notice the distinction: creating a Payment Link is not observing a payment, and observing a payment is not yet attribution.”

### 3:05–3:40 — Observe, attribute, recover, replay

“Razorpay sends `payment_link.paid` through the public tunnel. RecoverOS verifies the signed webhook before deduplication, resolves the Payment Link back through execution, action, decision, and case, and records strong direct-action attribution.

Only now does the case become RECOVERED and the ₹8,999 reach the operational metrics. The final stage is Learn and Audit: this immutable replay preserves the decision, policy, human authority, execution, provider evidence, attribution, feedback, and ordered events.”

### 3:40–4:05 — Correctly choosing not to intervene

“A recovery system also needs the judgment not to act. This customer has an active Promise-to-Pay. RecoverOS suppresses competing contacts and retries, keeps the durable promise check, and selects WAIT instead of creating another Payment Link. Quiet hours, opt-out, contact and retry caps, deadlines, and terminal states provide the same kind of deterministic protection.”

### 4:05–4:32 — Measured batch outcome

“This page is SYNTHETIC HELD-OUT EVALUATION—not production revenue. Across five seeds, 25,000 generated cases and 3,750 held-out cases per strategy, No Recovery reaches 22.1%, Rules 36.1%, and RecoverOS Full NBA 42.4% recovery. Mean net recovered value is about ₹5.39 lakh for Full NBA versus ₹2.73 lakh for No Recovery. Under identical simulated capacity constraints, NERV-Greedy also produces 68.7% more expected NERV than first-come-first-served.”

### 4:32–4:50 — Reliability

“The Resilience Lab is a deterministic fault simulation. Ten duplicate webhook deliveries still produce one bounded business effect, and a decision-service timeout fails closed before provider execution. One lesson from building this was that signatures must be verified before reserving a webhook event ID; otherwise an invalid request can poison deduplication.”

### 4:50–5:00 — Close

“So the outcome is measurable recovery across a batch, bounded autonomous action, compliant human escalation, explicit stopping rules, durable execution, evidence-based attribution, and a complete audit trail. RecoverOS does not merely retry payments—it decides when recovery is worth attempting, executes safely, and proves the result.”

## 2. Parallel UI action plan

### Time: 0:00–0:22

**Say:** The “problem” transcript block.

**Do:**

1. Start at `http://localhost:3000/`.
2. Point to the hero text: **Recover more revenue. Prove every decision.**
3. Briefly sweep across **Revenue at risk**, **Recovered**, **Agent-attributed**, **Natural recovery**, **Awaiting review**, and **Scheduled**.
4. Click **Launch guided demo**.

**Do not:** Read the current operational amounts aloud; they depend on the database state.

### Time: 0:22–0:42

**Say:** The “autonomous safe case” transcript block.

**Do:**

1. On `/demo`, point briefly to the **RAZORPAY TEST MODE — CONNECTED** provider card.
2. Find **Checkout abandonment — ₹2,499 payment-friction abandonment**.
3. Click its **Create & run decision** button.
4. Wait for **GUIDED LIVE JOURNEY** to appear.
5. Point to the completed risk/decision/gate/policy stages. If scheduling/link creation completes within this block, point to those completed stages too.

**Wait for:** The button to stop showing **Running real workflow…**.

**Do not:** Open or pay this first case’s Payment Link. It is only the brief autonomous contrast.

### Time: 0:42–1:35

**Say:** The “main ₹8,999 case and Next-Best-Action” transcript block.

**Do:**

1. On `/demo`, find **₹8,999 High-value checkout**.
2. Click **Create & run decision**.
3. Wait for its new **GUIDED LIVE JOURNEY** and **Human approval is required** notice.
4. Click **Open full case →**.
5. On `/recovery/<new-case-id>`, point to the state **ESCALATED** and the nine-stage pipeline.
6. Scroll normally to **DIAGNOSE · DETERMINISTIC**; point at failure category, confidence, recoverability, and observable customer state.
7. Scroll to **DECIDE · ML PREDICTION + DETERMINISTIC OPTIMIZER**.
8. Point, in order, to **P(recovery | WAIT)**, **Selected action P**, **Incremental uplift**, and **Expected NERV**.
9. Point to the highlighted selected row in the candidate table and then the NERV waterfall.
10. Scroll to **BOUND · ELIGIBILITY + ECONOMIC GATE + POLICY**; point to gate **ALLOW** and policy **ESCALATE / HIGH VALUE APPROVAL**.

**Wait for:** The case page to display decision candidates. If it does not, reload once.

**Do not:** Quote the per-case probabilities or NERV from memory. Let the current persisted values remain visible while using the fixed spoken explanation.

### Time: 1:35–2:05

**Say:** The “human authority” transcript block.

**Do:**

1. Click **Operations** in the top navigation.
2. Find the top/current ₹8,999 checkout-abandonment card. Confirm its deadline is current—not the canonical 2035 fixture if that fixture is present.
3. Point to **Recommended**, **Expected NERV**, **Action / natural P**, **Incremental uplift**, and **Why this needs a human**.
4. Click **Approve safely**.
5. In **Operator ID**, keep or enter `demo-operator`, then confirm.
6. In **Review notes**, enter `High-value Test Mode recovery approved for demo`, then confirm.
7. Point to **Human approval recorded**, **Fresh case version**, **Merchant policy version**, and **Fresh authorization: APPROVED**.

**Wait for:** The green/pass reauthorization result and schedule ID.

**Do not:** Click **Reject**, **Defer**, or **Stop case**.

### Time: 2:05–2:35

**Say:** The “durable execution” transcript block.

**Do:**

1. Use browser **Back** to return to the high-value case page.
2. Reload the page once.
3. Scroll normally to **AUTHORIZE · FRESH AT EXECUTION** and point to the human authority and reauthorization result.
4. Continue to **EXECUTE · DURABLE WORK**.
5. If it still says **Not scheduled**, reload after three seconds.
6. Point to action, attempt count, idempotency key, and provider reference.
7. Confirm the button says **Open Razorpay Test Payment — TEST MODE · NO REAL MONEY**.

**Wait for:** A provider reference and the Test Payment button. Allow up to 10 seconds with manual reloads.

### Time: 2:35–3:05

**Say:** The “Test Mode payment” transcript block, then remain silent while completing the form.

**Do:**

1. Click **Open Razorpay Test Payment**. It opens a new tab.
2. Confirm the hosted page visibly indicates Test Mode and ₹8,999.
3. Select **Netbanking** and any available bank.
4. Proceed to the Razorpay mock bank page.
5. Select **Success**.
6. Wait for the payment-success confirmation.
7. Return to the RecoverOS case tab.

**Wait for:** Razorpay’s success confirmation before leaving the tab.

**Do not:** Use Live Mode, real banking credentials, or click **Failure**. Razorpay’s hosted wording can vary; during rehearsal, confirm the exact Netbanking labels available for the account.

### Time: 3:05–3:40

**Say:** The “observe, attribute, recover, replay” transcript block.

**Do:**

1. Reload the RecoverOS case page every three seconds until the state is **RECOVERED**. Stop after five reloads.
2. Point to **₹8,999 recovered** in the case hero.
3. Scroll to **OBSERVE · PROVIDER EVIDENCE** and point to `payment_link.paid` in the webhook evidence.
4. Scroll to **ATTRIBUTE · EVIDENCE SCOPE**.
5. Trace the visible chain: **Provider payment → Reference → Execution → Action → Decision → RecoveryCase**.
6. Point to **DIRECT ACTION ATTRIBUTED**, **STRONG**, and **Feedback: Recorded**.
7. Scroll to **LEARN / AUDIT · IMMUTABLE HISTORY** and expand only the final **RECOVERY COMPLETED** event.

**Wait for:** `RECOVERED` and an attribution card. Do not say recovery completed before both appear.

### Time: 3:40–4:05

**Say:** The “correctly choosing not to intervene” transcript block.

**Do:**

1. Click **Demo** in the top navigation.
2. Find **Active Promise-to-Pay**.
3. Click **Create & run decision** and wait for the journey card.
4. Click **Open full case →**.
5. Scroll to **BOUND** and point to excluded contact/retry actions and the active-promise reason.
6. Point to the selected **WAIT** action.
7. Scroll to **PROMISE-TO-PAY LIFECYCLE** and point to **ACTIVE** and the durable check count.

**Do not:** Click **Simulate fulfilled** or **Simulate broken → re-decide** during the five-minute run.

### Time: 4:05–4:32

**Say:** The “measured batch outcome” transcript block.

**Do:**

1. Click **Evaluation** in the top navigation.
2. Point first to **SYNTHETIC HELD-OUT EVALUATION**.
3. Point to **5 seeds**, **25,000 generated cases**, and **3,750 held-out / strategy**.
4. Sweep across the strategy graph/table from **No Recovery** to **RecoverOS Full NBA**.
5. Point to **NERV-Greedy vs FCFS** and **68.7%**.
6. Briefly point to **Ablation Evidence** without explaining individual ablations.

**Do not:** Call these values live, operational, production, or causal recovery results.

### Time: 4:32–4:50

**Say:** The “reliability” transcript block.

**Do:**

1. Click **Reliability** in the top navigation.
2. Click **Duplicate Webhook ×10**.
3. Point to **PASS**, **Events delivered: 10**, and **Duplicates blocked: 9**.
4. Click **Decision Service Timeout**.
5. Point to **PASS** and **External effects: 0**.

**Wait for:** Each result card to show **PASS** before running the next scenario.

**Do not:** Describe the lab as a real Razorpay outage or production chaos test.

### Time: 4:50–5:00

**Say:** The closing transcript block.

**Do:**

1. Leave the final Reliability **PASS** result visible.
2. Stop interacting with the UI.
3. Finish facing the judges/camera rather than reading the screen.

## 3. Pre-demo staging and contingency plan

### Ten minutes before presenting

1. From Git Bash in the repository root, run:

   ```bash
   docker compose up -d --build
   sh scripts/demo-preflight.sh
   ```

2. Confirm the preflight prints all three `PASS` lines and opens the expected URL recommendation.
3. Open these tabs before the timer starts:
   - `http://localhost:3000/`
   - Razorpay Dashboard **Test Mode → Webhooks**, for delivery troubleshooting only
4. Set browser zoom so the candidate table and pipeline remain readable without horizontal scrolling.
5. Close old Razorpay hosted checkout tabs so the newly opened ₹8,999 link is unmistakable.
6. Rehearse the hosted Test Mode payment once. [Razorpay’s Test Payment Link flow](https://razorpay.com/docs/payments/payment-links/create/?preferred-country=IN) allows selecting a payment method and choosing a simulated success; Netbanking avoids entering real payment credentials. No real money is moved.
7. If old dynamic cases make Operations ambiguous, restore the demo baseline before—not during—the presentation:

   ```bash
   REQUIRE_DEMO_RESET=YES sh scripts/demo-reset.sh
   ```

### Timing safeguards

- The worker polls about once per second; budget up to 10 seconds and reload manually on the server-rendered case page.
- The Demo journey card polls every three seconds, but the full case page does not auto-refresh.
- Operations removes an approved card from the queue; use browser Back to return to the exact case you opened before approval.
- The Operations seed can contain another ₹8,999 canonical fixture with a 2035 deadline. Approve the newly created case with the current recovery deadline.
- Speak the fixed evaluation values only on `/evaluation`. Never use changing Command Center totals as scripted numbers.

### If the provider or webhook is slow

1. If no Payment Link appears after 10 seconds, reload the case once more. If it still does not appear, do not improvise a recovery claim; say: “The durable schedule remains visible, and this case has not been counted as recovered.” Continue to PTP/Evaluation/Reliability.
2. If Razorpay payment succeeds but `RECOVERED` does not appear after 15 seconds, leave the case showing **Outcome pending**, briefly open the Razorpay Test Mode webhook delivery tab only to confirm whether delivery is pending, and continue. Never use manual attribution as a substitute in the judged path.
3. Keep a bookmarked, previously completed Test Mode case from rehearsal as a replay-only fallback. Label it explicitly as a prior Test Mode run, not the live case just created.
4. If a scenario returns an error, do not repeatedly click its button. Move to the next prepared evidence surface.

## 4. Final judge-coverage check

| Criterion | Visible demo moment | What the judge learns |
|---|---|---|
| Problem taste | 0:00 problem framing; 3:40 active PTP | Blind retry/contact ignores natural recovery, economics, fatigue, promises, and customer protections |
| Build quality | Persisted case, Operations approval, worker-created Razorpay link, signed webhook, attribution, replay | This is a running stateful workflow, not a message mock-up or static prediction screen |
| AI judgment | Decision probabilities, natural baseline, uplift, NERV, Next-Best-Action; spoken control principle | Statistical models estimate outcomes while deterministic eligibility, economics, policy, and authority control execution |
| Failure recovery | Duplicate Webhook ×10 and Decision Service Timeout PASS; signature-before-dedupe lesson | Duplicate delivery and inference failure remain bounded and fail closed |
| Challenge bar | Autonomous safe case; high-value escalation; PTP stopping rule; held-out strategy comparison; recovered attribution | The demo covers measured batch recovery, compliant escalation, stopping behavior, and evidence-based recovery |
| Submission quality | One connected nine-stage story, explicit evidence labels, honest Test Mode/synthetic boundaries, immutable replay | Claims are polished, traceable, and scoped to what the system actually proves |

### Required outcome checklist

- [x] Revenue problem and why blind retries are insufficient
- [x] Detect and deterministic diagnosis
- [x] Nine-stage pipeline as the narrative backbone
- [x] Natural/action probability, uplift, NERV, and NBA
- [x] AI/statistical prediction versus deterministic authorization
- [x] Economic gate and merchant policy
- [x] Autonomous safe-case behavior
- [x] Human-in-the-loop high-value escalation
- [x] Fresh reauthorization
- [x] Real Razorpay Test Mode Payment Link execution
- [x] Link created ≠ payment observed ≠ revenue attributed
- [x] Signed `payment_link.paid`, strong direct attribution, and `RECOVERED`
- [x] Promise-to-Pay suppression and WAIT
- [x] Synthetic held-out baseline comparison
- [x] Deterministic reliability evidence and engineering lesson
- [x] Immutable replay and feedback evidence
- [x] Recovered revenue, bounded escalation, stopping rules, and audit trail
