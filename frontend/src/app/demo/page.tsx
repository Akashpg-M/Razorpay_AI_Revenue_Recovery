import Link from "next/link";
import { Nav } from "@/components/nav";
import { ScenarioLab } from "@/components/scenario-lab";
import { RecoveryPipeline } from "@/components/recovery-pipeline";
import { PresenterGuide } from "@/components/presenter-guide";
import { backendJSON } from "@/lib/api";

type Provider = { selected_provider: string; mode: string; configured: boolean; reachable: boolean; authenticated: boolean; webhook_verification_configured: boolean; external_webhook_delivery_configured: boolean };
const proofs = [
  ["How is risk detected?", "Normalized payment failures and checkout abandonment create one stateful case.", "/recoveries", "LIVE CASES"],
  ["Where is AI used?", "Versioned models estimate action-conditioned and natural recovery probabilities.", "/evaluation", "MODEL EVIDENCE"],
  ["What remains deterministic?", "Eligibility, NERV arithmetic, economic gates, policy and state transitions.", "/recoveries", "CASE DEEP DIVE"],
  ["Can it act autonomously?", "Yes—only low-risk actions that pass all limits. High-value work requires human authority.", "/operations", "AUTHORITY QUEUE"],
  ["Does Razorpay really work?", "Test Mode API authentication, Payment Links and signed webhook ingestion are exposed separately.", "/observability", "PROVIDER HEALTH"],
  ["How is recovery proven?", "A payment observation is linked back through execution, action and decision lineage.", "/recoveries", "ATTRIBUTION"],
  ["How was it evaluated?", "Frozen multi-seed held-out simulations compare identical populations and strategies.", "/evaluation", "EVALUATION"],
  ["What happens when it breaks?", "Executable fault scenarios preserve idempotency, leases, stale-decision safety and reconciliation.", "/resilience", "FAILURE STORIES"],
] as const;

export default async function Demo() {
  const p = await backendJSON<Provider>("/api/v1/integrations/razorpay/status"), razorpay = p?.selected_provider === "razorpay";
  return <><Nav /><PresenterGuide /><main className="shell"><section className="pageIntro demoIntro"><div><div className="eyebrow">JUDGE-FACING PRODUCT WALKTHROUGH</div><h1>Run it. Understand it.<br /><em>Prove every claim.</em></h1><p>The browser invokes real backend workflows. Every state transition, decision, authorization, provider reference and attribution shown here comes from persisted evidence.</p></div><aside className="providerCard"><small>{razorpay ? "RAZORPAY TEST MODE" : "DETERMINISTIC LOCAL MODE"}</small><strong>{razorpay ? p?.authenticated && p?.reachable ? "CONNECTED" : "NOT READY" : "READY OFFLINE"}</strong><span>Provider {p?.selected_provider ?? "unknown"}</span><span>Credentials {razorpay ? p?.configured ? "configured" : "missing" : "not required"}</span><span>Webhook HMAC {p?.webhook_verification_configured ? "ready" : "not configured"}</span><span>Public tunnel {p?.external_webhook_delivery_configured ? "configured" : "not configured"}</span></aside></section>
    <section className="demoModeTabs"><a href="#run"><b>RUN</b><span>Create and operate a real case</span></a><a href="#understand"><b>UNDERSTAND</b><span>See each internal boundary</span></a><a href="#proof-map"><b>PROVE</b><span>Answer reviewer questions with evidence</span></a></section>
    <div id="run"><div className="sectionHead"><div><span className="dataMode">RUN · LIVE OPERATIONAL / SYNTHETIC INPUT</span><h2>Choose a controlled business scenario</h2></div><span>Razorpay calls occur only when its action is scheduled and authorized</span></div><ScenarioLab /></div>
    <div id="understand"><RecoveryPipeline conceptual /></div>
    <section id="proof-map" className="panel proofMap"><div className="panelTitle"><div><div className="eyebrow">REVIEWER PROOF MAP</div><h2>Questions answered by operating evidence</h2></div><span>PROVE</span></div><div>{proofs.map(([question, answer, href, label]) => <article key={question}><small>{question}</small><p>{answer}</p><Link href={href}>{label} →</Link></article>)}</div></section>
    <section className="panel alignment"><div className="eyebrow">CLAIM BOUNDARIES</div><h2>What each evidence surface does—and does not—prove.</h2><div>{[["Operational", "Live persisted Test Mode/local cases; not production lift."], ["Synthetic", "Reproducible relative strategy performance; not a causal production claim."], ["Razorpay", "Test Mode integration and signed webhook handling; no real-money claim."], ["Resilience", "Deterministic harness invariants; provider-specific scope is labelled."]].map(([a, b]) => <span key={a}><i>✓</i><b>{a}</b><small>{b}</small></span>)}</div></section>
  </main></>;
}
