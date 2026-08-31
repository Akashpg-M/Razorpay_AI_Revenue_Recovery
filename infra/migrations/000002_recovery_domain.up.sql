CREATE TABLE merchants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    merchant_type TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE merchant_policies (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    objective TEXT NOT NULL CHECK (objective IN ('MAXIMIZE_NET_RECOVERY','MAXIMIZE_RETENTION','MINIMIZE_CONTACT','MINIMIZE_RECOVERY_COST','BALANCED')),
    max_retries INTEGER NOT NULL CHECK (max_retries >= 0),
    max_contacts_per_day INTEGER NOT NULL CHECK (max_contacts_per_day >= 0),
    max_contacts_per_week INTEGER NOT NULL CHECK (max_contacts_per_week >= 0),
    min_contact_interval_minutes INTEGER NOT NULL CHECK (min_contact_interval_minutes >= 0),
    recovery_window_hours INTEGER NOT NULL CHECK (recovery_window_hours > 0),
    quiet_hours JSONB NOT NULL DEFAULT '{}',
    allowed_actions TEXT[] NOT NULL,
    allowed_channels TEXT[] NOT NULL,
    high_value_threshold_minor BIGINT NOT NULL CHECK (high_value_threshold_minor >= 0),
    low_confidence_threshold DOUBLE PRECISION NOT NULL CHECK (low_confidence_threshold BETWEEN 0 AND 1),
    minimum_economic_value_minor BIGINT NOT NULL DEFAULT 0,
    maximum_incentive_minor BIGINT NOT NULL DEFAULT 0 CHECK (maximum_incentive_minor >= 0),
    requires_high_value_human_approval BOOLEAN NOT NULL DEFAULT TRUE,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, version)
);

CREATE TABLE customers (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    external_id TEXT NOT NULL,
    contact JSONB NOT NULL DEFAULT '{}',
    opted_out BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, external_id)
);

CREATE TABLE customer_recovery_profiles (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL REFERENCES customers(id),
    successful_payments INTEGER NOT NULL DEFAULT 0 CHECK (successful_payments >= 0),
    failed_payments INTEGER NOT NULL DEFAULT 0 CHECK (failed_payments >= 0),
    subscription_tenure_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_tenure_days >= 0),
    promise_reliability DOUBLE PRECISION NOT NULL DEFAULT 0.5 CHECK (promise_reliability BETWEEN 0 AND 1),
    fatigue_score DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (fatigue_score BETWEEN 0 AND 1),
    features JSONB NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (customer_id, version)
);

CREATE TABLE model_versions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    feature_version TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    artifact_uri TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name, version)
);

CREATE TABLE recovery_cases (
    id TEXT PRIMARY KEY,
    leak_type TEXT NOT NULL CHECK (leak_type IN ('FAILED_SUBSCRIPTION','CHECKOUT_ABANDONMENT')),
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    customer_id TEXT NOT NULL REFERENCES customers(id),
    amount_at_risk_minor BIGINT NOT NULL CHECK (amount_at_risk_minor > 0),
    currency CHAR(3) NOT NULL,
    source_reference TEXT NOT NULL,
    source_status TEXT NOT NULL DEFAULT '',
    failure_or_leak_context JSONB NOT NULL DEFAULT '{}',
    customer_context_snapshot JSONB NOT NULL DEFAULT '{}',
    merchant_policy_snapshot JSONB NOT NULL DEFAULT '{}',
    current_state TEXT NOT NULL CHECK (current_state IN ('DETECTED','DIAGNOSING','ACTION_PENDING','POLICY_REVIEW','SCHEDULED','EXECUTING','WAITING_OUTCOME','REASSESSING','RECOVERED','ESCALATED','EXHAUSTED','STOPPED')),
    recovery_deadline TIMESTAMPTZ NOT NULL,
    recovered_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (recovered_amount_minor >= 0),
    attribution_status TEXT NOT NULL DEFAULT 'PENDING',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, source_reference)
);
CREATE INDEX recovery_cases_queue_idx ON recovery_cases (merchant_id, current_state, recovery_deadline);
CREATE INDEX recovery_cases_customer_idx ON recovery_cases (customer_id, created_at DESC);

CREATE TABLE recovery_events (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    event_type TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    actor JSONB NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    model_version TEXT,
    correlation_id TEXT NOT NULL,
    UNIQUE (case_id, sequence)
);
CREATE INDEX recovery_events_case_time_idx ON recovery_events (case_id, occurred_at, sequence);
CREATE INDEX recovery_events_correlation_idx ON recovery_events (correlation_id);

CREATE TABLE policy_decisions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    action_id TEXT,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVED','DENIED','ESCALATED','STOPPED')),
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    snapshot JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE recovery_actions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    action_type TEXT NOT NULL CHECK (action_type IN ('WAIT','RETRY_NOW','RETRY_LATER','SEND_REMINDER','SEND_PAYMENT_LINK','SEND_CHECKOUT_RECOVERY_LINK','REQUEST_PAYMENT_METHOD_UPDATE','SUGGEST_ALTERNATE_METHOD','WAIT_FOR_PROMISE_TO_PAY','RETENTION_ACTION','ESCALATE_TO_HUMAN','STOP')),
    status TEXT NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}',
    idempotency_key TEXT NOT NULL UNIQUE,
    policy_decision_id TEXT REFERENCES policy_decisions(id),
    scheduled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE policy_decisions ADD CONSTRAINT policy_decisions_action_fk FOREIGN KEY (action_id) REFERENCES recovery_actions(id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE action_predictions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    action_id TEXT REFERENCES recovery_actions(id),
    action_type TEXT NOT NULL,
    recovery_probability DOUBLE PRECISION NOT NULL CHECK (recovery_probability BETWEEN 0 AND 1),
    natural_recovery_probability DOUBLE PRECISION NOT NULL CHECK (natural_recovery_probability BETWEEN 0 AND 1),
    incremental_uplift DOUBLE PRECISION NOT NULL CHECK (incremental_uplift BETWEEN -1 AND 1),
    expected_net_value_minor BIGINT NOT NULL,
    model_version_id TEXT REFERENCES model_versions(id),
    feature_version TEXT NOT NULL,
    explanation JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE executions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    action_id TEXT NOT NULL REFERENCES recovery_actions(id),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('SUCCEEDED','FAILED','TIMED_OUT','REJECTED_BY_PROVIDER','CANCELLED','NO_RESPONSE','RECOVERY_CONFIRMED','OUTCOME_PENDING')),
    provider_reference TEXT NOT NULL DEFAULT '',
    request JSONB NOT NULL DEFAULT '{}',
    response JSONB NOT NULL DEFAULT '{}',
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    UNIQUE (action_id, attempt)
);

CREATE TABLE promises_to_pay (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    customer_id TEXT NOT NULL REFERENCES customers(id),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE','FULFILLED','BROKEN','EXPIRED','CANCELLED')),
    due_at TIMESTAMPTZ NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    source JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX one_active_promise_per_case_idx ON promises_to_pay(case_id) WHERE status='ACTIVE';

CREATE TABLE webhook_events (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('RECEIVED','PROCESSING','PROCESSED','FAILED','IGNORED')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    UNIQUE (provider, provider_event_id)
);

CREATE TABLE evaluation_runs (
    id TEXT PRIMARY KEY,
    simulation_version TEXT NOT NULL,
    seed BIGINT NOT NULL,
    dataset_size INTEGER NOT NULL CHECK (dataset_size > 0),
    split_identifiers JSONB NOT NULL,
    model_version_id TEXT REFERENCES model_versions(id),
    feature_version TEXT NOT NULL,
    strategy_version TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE FUNCTION reject_recovery_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'recovery_events is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER recovery_events_immutable
BEFORE UPDATE OR DELETE ON recovery_events
FOR EACH ROW EXECUTE FUNCTION reject_recovery_event_mutation();

UPDATE platform_metadata SET value='phase_4' WHERE key='schema_version';
