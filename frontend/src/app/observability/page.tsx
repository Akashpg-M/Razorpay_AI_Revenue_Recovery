import { Nav } from "@/components/nav";
import { backendJSON } from "@/lib/api";

type Alert = { code: string; severity: string; message: string };
type Snapshot = {
  generated_at: string;
  schema_version: string;
  queue: { pending: number; running: number; failed: number; max_lag_seconds: number };
  execution: { succeeded: number; failed: number; timed_out: number; retrying: number };
  recovery: {
    active_cases: number;
    recovered_cases: number;
    escalated_cases: number;
    stopped_cases: number;
    expired_promises: number;
  };
  // Older deployments may encode a nil Go slice as JSON null.
  alerts: Alert[] | null;
};

function Intro() {
  return (
    <section className="pageIntro">
      <div className="eyebrow">OPERATIONAL HEALTH</div>
      <h1>See the system.<br /><em>Before it surprises you.</em></h1>
      <p>Durable queue lag, execution outcomes, recovery states, promises, schema posture, and derived alerts from persisted truth.</p>
    </section>
  );
}

export default async function Observability() {
  const snapshot = await backendJSON<Snapshot>("/api/v1/observability");
  if (!snapshot) {
    return <><Nav /><main className="shell"><Intro /><div className="notice">Observability snapshot unavailable. Check backend readiness and migrations.</div></main></>;
  }

  const alerts = Array.isArray(snapshot.alerts) ? snapshot.alerts : [];

  return (
    <>
      <Nav />
      <main className="shell">
        <Intro />
        <section className="kpis">
          <article><small>Queue pending / running</small><strong>{snapshot.queue.pending} / {snapshot.queue.running}</strong><span>Max lag {Math.round(snapshot.queue.max_lag_seconds)}s</span></article>
          <article><small>Execution success / failed</small><strong>{snapshot.execution.succeeded} / {snapshot.execution.failed}</strong><span>{snapshot.execution.retrying} retrying · {snapshot.execution.timed_out} timeout</span></article>
          <article><small>Active / recovered</small><strong>{snapshot.recovery.active_cases} / {snapshot.recovery.recovered_cases}</strong><span>{snapshot.recovery.escalated_cases} escalated</span></article>
          <article><small>Schema</small><strong>{snapshot.schema_version}</strong><span>{new Date(snapshot.generated_at).toLocaleString()}</span></article>
        </section>
        <section className="panel alertPanel">
          <div className="panelTitle"><h2>Active alerts</h2><span>{alerts.length}</span></div>
          {alerts.length ? alerts.map((alert) => (
            <div className={`alert ${alert.severity}`} key={alert.code}>
              <b>{alert.code.replaceAll("_", " ")}</b><span>{alert.message}</span>
            </div>
          )) : <div className="healthy">No thresholds breached</div>}
        </section>
      </main>
    </>
  );
}
