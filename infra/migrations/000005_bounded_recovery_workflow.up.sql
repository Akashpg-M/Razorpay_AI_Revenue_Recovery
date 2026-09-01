ALTER TABLE action_predictions
    ADD COLUMN case_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN context_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN natural_model_version TEXT NOT NULL DEFAULT '';

CREATE TABLE natural_recovery_predictions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    case_version BIGINT NOT NULL,
    context_version TEXT NOT NULL,
    probability DOUBLE PRECISION NOT NULL CHECK (probability BETWEEN 0 AND 1),
    model_version TEXT NOT NULL,
    feature_version TEXT NOT NULL,
    predicted_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE recovery_decisions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    case_version BIGINT NOT NULL,
    optimizer_version TEXT NOT NULL,
    merchant_objective TEXT NOT NULL,
    context_version TEXT NOT NULL,
    outcome_model_version TEXT NOT NULL,
    natural_model_version TEXT NOT NULL,
    cost_model_version TEXT NOT NULL,
    selected_action TEXT NOT NULL,
    selected_nerv_minor BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX recovery_decisions_case_idx ON recovery_decisions(case_id,created_at DESC);

CREATE TABLE recovery_decision_candidates (
    id TEXT PRIMARY KEY,
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    action TEXT NOT NULL,
    action_probability DOUBLE PRECISION NOT NULL CHECK(action_probability BETWEEN 0 AND 1),
    natural_probability DOUBLE PRECISION NOT NULL CHECK(natural_probability BETWEEN 0 AND 1),
    incremental_uplift DOUBLE PRECISION NOT NULL CHECK(incremental_uplift BETWEEN -1 AND 1),
    gross_incremental_value_minor BIGINT NOT NULL,
    channel_cost_minor BIGINT NOT NULL,
    incentive_cost_minor BIGINT NOT NULL,
    operational_cost_minor BIGINT NOT NULL,
    fatigue_penalty_minor BIGINT NOT NULL,
    risk_penalty_minor BIGINT NOT NULL,
    nerv_minor BIGINT NOT NULL,
    objective_score_minor BIGINT NOT NULL,
    ranking_position INTEGER NOT NULL,
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    UNIQUE(decision_id,action)
);

CREATE TABLE economic_gate_evaluations (
    id TEXT PRIMARY KEY,
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    action TEXT NOT NULL,
    nerv_minor BIGINT NOT NULL,
    threshold_minor BIGINT NOT NULL,
    result TEXT NOT NULL CHECK(result IN('ALLOW','BLOCK')),
    reason_code TEXT NOT NULL,
    gate_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE policy_evaluations (
    id TEXT PRIMARY KEY,
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    economic_gate_id TEXT NOT NULL REFERENCES economic_gate_evaluations(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    case_version BIGINT NOT NULL,
    selected_action TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    result TEXT NOT NULL CHECK(result IN('APPROVE','DENY','ESCALATE','STOP')),
    reason_codes TEXT[] NOT NULL,
    checks JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE scheduled_actions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    policy_evaluation_id TEXT NOT NULL REFERENCES policy_evaluations(id),
    recovery_action_id TEXT NOT NULL REFERENCES recovery_actions(id),
    action TEXT NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}',
    scheduled_for TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK(status IN('PENDING','CLAIMED','EXECUTING','OBSERVATION_PENDING','OBSERVATION_CLAIMED','OBSERVED','FAILED','RETRY_PENDING','CANCELLED','SUPERSEDED')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    idempotency_key TEXT NOT NULL UNIQUE,
    case_version_at_schedule BIGINT NOT NULL,
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
CREATE INDEX scheduled_actions_due_idx ON scheduled_actions(status,(COALESCE(next_retry_at,scheduled_for)));

ALTER TABLE executions
    ADD COLUMN scheduled_action_id TEXT REFERENCES scheduled_actions(id),
    ADD COLUMN idempotency_key TEXT,
    ADD COLUMN failure_class TEXT,
    ADD COLUMN retryable BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX executions_idempotency_idx ON executions(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE email_deliveries (
    id TEXT PRIMARY KEY,
    scheduled_action_id TEXT NOT NULL REFERENCES scheduled_actions(id),
    idempotency_key TEXT NOT NULL UNIQUE,
    recipient_reference TEXT NOT NULL,
    template_name TEXT NOT NULL,
    safe_payload JSONB NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

-- Development-safe retry command outbox. This records a requested retry but does
-- not claim that Razorpay exposes a direct manual retry API.
CREATE TABLE retry_requests (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK(status IN('CAPTURED','DISPATCHED','REJECTED')),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE customer_responses (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    response_type TEXT NOT NULL CHECK(response_type IN('ACKNOWLEDGEMENT','INTENT_TO_PAY','PROMISE_TO_PAY','OPT_OUT','PAYMENT_METHOD_ISSUE','UNRESOLVED')),
    payload JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    correlation_id TEXT NOT NULL UNIQUE
);

CREATE FUNCTION reject_append_only_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER natural_predictions_immutable BEFORE UPDATE OR DELETE ON natural_recovery_predictions FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER decisions_immutable BEFORE UPDATE OR DELETE ON recovery_decisions FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER decision_candidates_immutable BEFORE UPDATE OR DELETE ON recovery_decision_candidates FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER economic_gates_immutable BEFORE UPDATE OR DELETE ON economic_gate_evaluations FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER policy_evaluations_immutable BEFORE UPDATE OR DELETE ON policy_evaluations FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();

UPDATE platform_metadata SET value='phase_16' WHERE key='schema_version';
