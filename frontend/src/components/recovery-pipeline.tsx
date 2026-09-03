type Row = Record<string, unknown>;

export type PipelineEvidence = {
  case?: Row;
  events?: Row[];
  decisions?: Row[];
  economic_gates?: Row[];
  policy_evaluations?: Row[];
  human_reviews?: Row[];
  executions?: Row[];
  webhook_events?: Row[];
  attributions?: Row[];
  feedback_records?: Row[];
};

const definitions = [
  ["Detect", "Revenue-risk signal normalized", "deterministic", "overview"],
  ["Diagnose", "Failure category and recoverability", "deterministic", "diagnosis"],
  ["Decide", "Action-conditioned probability + NERV", "ML + optimizer", "decision"],
  ["Bound", "Eligibility, economics and policy", "deterministic", "bounds"],
  ["Authorize", "Autonomous or human authority", "policy / human", "authorization"],
  ["Execute", "Durable idempotent provider work", "deterministic", "execution"],
  ["Observe", "Signed provider outcome", "external evidence", "observation"],
  ["Attribute", "Payment-to-decision lineage", "deterministic", "attribution"],
  ["Learn / audit", "Immutable history and feedback", "audit", "replay"],
] as const;

export function RecoveryPipeline({ evidence, conceptual = false }: { evidence?: PipelineEvidence; conceptual?: boolean }) {
  const e = evidence ?? {};
  const policy = e.policy_evaluations?.at(-1);
  const done = conceptual ? definitions.map(() => false) : [
    Boolean(e.case),
    Boolean((e.decisions?.at(-1)?.decision_context as Row | undefined)?.diagnosis ?? e.case?.failure_or_leak_context),
    Boolean(e.decisions?.length),
    Boolean(e.economic_gates?.length && e.policy_evaluations?.length),
    policy?.result === "APPROVE" || Boolean(e.human_reviews?.some((x) => x.decision === "APPROVE" || x.status === "APPROVED")),
    Boolean(e.executions?.length),
    Boolean(e.webhook_events?.some((item) => item.event_type === "payment_link.paid") || e.attributions?.length),
    Boolean(e.attributions?.length),
    Boolean(e.events?.length || e.feedback_records?.length),
  ];
  const current = done.findIndex((value) => !value);
  return <section className="pipeline panel" aria-label="Recovery pipeline">
    <div className="panelTitle"><div><div className="eyebrow">PERSISTED RECOVERY PIPELINE</div><h2>Nine stages, one evidence chain</h2></div><span>{conceptual ? "SYSTEM MAP" : `${done.filter(Boolean).length}/9 evidenced`}</span></div>
    <div className="pipelineTrack">{definitions.map(([name, detail, kind, anchor], index) =>
      <a href={`#${anchor}`} className={conceptual ? "pipelineStep" : `pipelineStep ${done[index] ? "complete" : index === current ? "current" : "pending"}`} key={name}>
        <i>{conceptual ? index + 1 : done[index] ? "✓" : index + 1}</i><b>{name}</b><small>{detail}</small><code>{kind}</code>
      </a>)}</div>
  </section>;
}
