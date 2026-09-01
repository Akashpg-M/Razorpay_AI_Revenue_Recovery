"use client";

import { useState } from "react";
import { publicBackend } from "@/lib/api";

type Result = {
  resilience_run_id: string;
  result: {
    scenario: string; passed: boolean; external_call_count: number;
    provider_effect_count: number; execution_attempt_count: number;
    events_delivered: number; duplicates_blocked: number;
    reconciliation_events: number; suppression_reason?: string;
  };
};

const scenarios = [
  ["Duplicate Webhook ×10", "duplicate_webhook"], ["Worker Crash", "worker_crash"],
  ["Razorpay Timeout", "razorpay_timeout"], ["Invalid Model Output", "invalid_model_output"],
  ["Duplicate Job", "duplicate_job"], ["Out-of-Order Event", "out_of_order_event"],
  ["Provider Success + Response Lost", "provider_success_response_lost"],
  ["Expired Worker Lease", "expired_worker_lease"], ["Redis Unavailable", "redis_unavailable"],
  ["Stale NBA Decision", "stale_nba_decision"],
  ["Customer Pays Before Scheduled Action", "customer_pays_before_scheduled_action"],
];

export function ResilienceLab() {
  const [running, setRunning] = useState("");
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState("");
  async function run(id: string) {
    setRunning(id); setError(""); setResult(null);
    try {
      const response = await fetch(`${publicBackend}/api/v1/resilience/scenarios/${id}/run`, { method: "POST" });
      const body = await response.json();
      if (!response.ok) throw new Error(body?.error?.message ?? body?.error ?? "Scenario failed");
      setResult(body);
    } catch (value) { setError(value instanceof Error ? value.message : "Scenario failed"); }
    finally { setRunning(""); }
  }
  const metrics = result ? [
    ["Events delivered", result.result.events_delivered], ["Worker attempts", result.result.execution_attempt_count],
    ["Provider calls", result.result.external_call_count], ["External effects", result.result.provider_effect_count],
    ["Duplicates blocked", result.result.duplicates_blocked], ["Reconciliations", result.result.reconciliation_events],
  ] : [];
  return <div className="labGrid">
    <section className="panel"><div className="eyebrow">ACTUAL BACKEND FAULT INJECTION</div><h2>Choose a failure mode</h2><p className="muted">Each control invokes the Go worker harness and records the result in PostgreSQL. No counters are animated locally.</p><div className="scenarioGrid">{scenarios.map(([label, id]) => <button key={id} onClick={() => run(id)} disabled={!!running}>{running === id ? "Injecting…" : label}</button>)}</div></section>
    <section className="panel resultPanel"><div className="eyebrow">LATEST RUN</div>{error && <div className="error">{error}</div>}{!result && !error && <p className="muted">Run a scenario to inspect attempts, effects, and the protected invariant.</p>}{result && <><div className={result.result.passed ? "verdict pass" : "verdict fail"}>{result.result.passed ? "PASS" : "FAIL"}</div><h2>{result.result.scenario.replaceAll("_", " ")}</h2><div className="metricRows">{metrics.map(([label, value]) => <span key={label}>{label} <b>{value}</b></span>)}</div><div className="invariant"><small>INVARIANT</small><strong>At-least-once workflow; no uncontrolled duplicate side effect</strong></div><code className="runId">run / {result.resilience_run_id}</code></>}</section>
  </div>;
}
