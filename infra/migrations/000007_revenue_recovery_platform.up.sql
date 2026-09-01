ALTER TABLE promises_to_pay
    ADD COLUMN promised_amount_minor BIGINT CHECK(promised_amount_minor IS NULL OR promised_amount_minor > 0),
    ADD COLUMN extractor_version TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN extraction_timestamp TIMESTAMPTZ,
    ADD COLUMN source_response_id TEXT REFERENCES customer_responses(id),
    ADD COLUMN fulfilled_at TIMESTAMPTZ,
    ADD COLUMN broken_at TIMESTAMPTZ,
    ADD COLUMN expired_at TIMESTAMPTZ,
    ADD COLUMN cancelled_at TIMESTAMPTZ,
    ADD COLUMN verification_reference TEXT;
CREATE UNIQUE INDEX promises_source_response_idx ON promises_to_pay(source_response_id) WHERE source_response_id IS NOT NULL;

CREATE TABLE promise_events (
    id TEXT PRIMARY KEY,
    promise_id TEXT NOT NULL REFERENCES promises_to_pay(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    from_status TEXT,
    to_status TEXT NOT NULL CHECK(to_status IN('ACTIVE','FULFILLED','BROKEN','EXPIRED','CANCELLED')),
    reason_code TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL,
    correlation_id TEXT NOT NULL,
    UNIQUE(promise_id,correlation_id)
);

CREATE TABLE promise_checks (
    id TEXT PRIMARY KEY,
    promise_id TEXT NOT NULL UNIQUE REFERENCES promises_to_pay(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    scheduled_for TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK(status IN('PENDING','CLAIMED','COMPLETED','CANCELLED')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    result TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX promise_checks_due_idx ON promise_checks(status,scheduled_for);

CREATE TABLE merchant_optimization_profiles (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    objective TEXT NOT NULL CHECK(objective IN('MAXIMIZE_NET_RECOVERY','MAXIMIZE_RETENTION','MINIMIZE_CONTACT','MINIMIZE_RECOVERY_COST','BALANCED')),
    revenue_weight_bps INTEGER NOT NULL CHECK(revenue_weight_bps BETWEEN 0 AND 20000),
    retention_weight_bps INTEGER NOT NULL CHECK(retention_weight_bps BETWEEN 0 AND 20000),
    contact_penalty_weight_bps INTEGER NOT NULL CHECK(contact_penalty_weight_bps BETWEEN 0 AND 20000),
    cost_penalty_weight_bps INTEGER NOT NULL CHECK(cost_penalty_weight_bps BETWEEN 0 AND 20000),
    fatigue_penalty_weight_bps INTEGER NOT NULL CHECK(fatigue_penalty_weight_bps BETWEEN 0 AND 20000),
    risk_penalty_weight_bps INTEGER NOT NULL CHECK(risk_penalty_weight_bps BETWEEN 0 AND 20000),
    escalation_preference TEXT NOT NULL CHECK(escalation_preference IN('STANDARD','CONSERVATIVE','AGGRESSIVE')),
    allowed_actions TEXT[] NOT NULL,
    allowed_channels TEXT[] NOT NULL,
    minimum_nerv_minor BIGINT NOT NULL DEFAULT 0,
    discount_budget_minor BIGINT NOT NULL DEFAULT 0 CHECK(discount_budget_minor >= 0),
    human_review_budget_minor BIGINT NOT NULL DEFAULT 0 CHECK(human_review_budget_minor >= 0),
    configuration_version INTEGER NOT NULL CHECK(configuration_version > 0),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(merchant_id,configuration_version)
);

ALTER TABLE recovery_decisions
    ADD COLUMN merchant_profile_id TEXT REFERENCES merchant_optimization_profiles(id),
    ADD COLUMN merchant_profile_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN merchant_profile_snapshot JSONB NOT NULL DEFAULT '{}';

CREATE TABLE attribution_rule_configs (
    version TEXT PRIMARY KEY,
    retry_window_minutes INTEGER NOT NULL CHECK(retry_window_minutes > 0),
    direct_action_window_minutes INTEGER NOT NULL CHECK(direct_action_window_minutes > 0),
    email_assist_window_minutes INTEGER NOT NULL CHECK(email_assist_window_minutes > 0),
    ptp_window_minutes INTEGER NOT NULL CHECK(ptp_window_minutes > 0),
    created_at TIMESTAMPTZ NOT NULL
);
INSERT INTO attribution_rule_configs(version,retry_window_minutes,direct_action_window_minutes,email_assist_window_minutes,ptp_window_minutes,created_at)
VALUES('attribution-v1',1440,10080,4320,1440,NOW());

CREATE TABLE recovery_attributions (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    recovered_amount_minor BIGINT NOT NULL CHECK(recovered_amount_minor > 0),
    payment_reference TEXT NOT NULL,
    category TEXT NOT NULL CHECK(category IN('DIRECT_ACTION_ATTRIBUTED','RETRY_ATTRIBUTED','PTP_ATTRIBUTED','NATURAL_RECOVERY','UNKNOWN')),
    decision_id TEXT REFERENCES recovery_decisions(id),
    action_id TEXT REFERENCES recovery_actions(id),
    execution_id TEXT REFERENCES executions(id),
    promise_id TEXT REFERENCES promises_to_pay(id),
    evidence JSONB NOT NULL,
    evidence_strength TEXT NOT NULL CHECK(evidence_strength IN('STRONG','MODERATE','WEAK','INSUFFICIENT')),
    rule_version TEXT NOT NULL REFERENCES attribution_rule_configs(version),
    supersedes_id TEXT REFERENCES recovery_attributions(id),
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(case_id,payment_reference)
);

CREATE TABLE feedback_records (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    case_version BIGINT NOT NULL,
    decision_id TEXT REFERENCES recovery_decisions(id),
    execution_id TEXT REFERENCES executions(id),
    attribution_id TEXT REFERENCES recovery_attributions(id),
    context_version TEXT NOT NULL,
    observable_context JSONB NOT NULL,
    eligible_actions JSONB NOT NULL,
    selected_action TEXT NOT NULL,
    action_probability DOUBLE PRECISION NOT NULL CHECK(action_probability BETWEEN 0 AND 1),
    natural_probability DOUBLE PRECISION NOT NULL CHECK(natural_probability BETWEEN 0 AND 1),
    incremental_uplift DOUBLE PRECISION NOT NULL CHECK(incremental_uplift BETWEEN -1 AND 1),
    predicted_nerv_minor BIGINT NOT NULL,
    actual_recovered BOOLEAN NOT NULL,
    recovered_amount_minor BIGINT NOT NULL,
    intervention_cost_minor BIGINT NOT NULL,
    time_to_outcome_minutes BIGINT NOT NULL,
    policy_result TEXT NOT NULL,
    outcome_model_version TEXT NOT NULL,
    natural_model_version TEXT NOT NULL,
    optimizer_version TEXT NOT NULL,
    merchant_profile_version INTEGER NOT NULL,
    label_version TEXT NOT NULL,
    training_eligible BOOLEAN NOT NULL,
    exclusion_reasons TEXT[] NOT NULL DEFAULT '{}',
    environment TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(case_id,decision_id,execution_id,attribution_id)
);

CREATE TABLE training_dataset_versions (
    id TEXT PRIMARY KEY,
    version TEXT NOT NULL UNIQUE,
    source_start TIMESTAMPTZ NOT NULL,
    source_end TIMESTAMPTZ NOT NULL,
    row_count INTEGER NOT NULL CHECK(row_count >= 0),
    feature_version TEXT NOT NULL,
    label_version TEXT NOT NULL,
    source_model_versions JSONB NOT NULL,
    data_hash TEXT NOT NULL,
    exclusions JSONB NOT NULL,
    split_logic JSONB NOT NULL,
    manifest JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE model_registry_entries (
    id TEXT PRIMARY KEY,
    model_version TEXT NOT NULL UNIQUE,
    model_type TEXT NOT NULL,
    feature_version TEXT NOT NULL,
    training_dataset_version TEXT REFERENCES training_dataset_versions(version),
    algorithm TEXT NOT NULL,
    training_timestamp TIMESTAMPTZ NOT NULL,
    validation_metrics JSONB NOT NULL,
    calibration_metrics JSONB NOT NULL,
    artifact_uri TEXT NOT NULL,
    artifact_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE model_registry_status_events (
    id TEXT PRIMARY KEY,
    model_registry_id TEXT NOT NULL REFERENCES model_registry_entries(id),
    status TEXT NOT NULL CHECK(status IN('CANDIDATE','APPROVED','ACTIVE','RETIRED','REJECTED')),
    reason TEXT NOT NULL,
    actor JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE portfolio_priority_snapshots (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    action TEXT NOT NULL,
    amount_at_risk_minor BIGINT NOT NULL,
    expected_incremental_value_minor BIGINT NOT NULL,
    expected_nerv_minor BIGINT NOT NULL,
    urgency_bps INTEGER NOT NULL,
    recoverability_bps INTEGER NOT NULL,
    priority_score_minor BIGINT NOT NULL,
    rank INTEGER NOT NULL,
    explanation JSONB NOT NULL,
    algorithm_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(run_id,case_id)
);

CREATE TABLE budget_allocation_runs (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    algorithm_version TEXT NOT NULL,
    priority_run_id TEXT NOT NULL,
    budget JSONB NOT NULL,
    totals JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE budget_allocations (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES budget_allocation_runs(id),
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    action TEXT NOT NULL,
    expected_incremental_value_minor BIGINT NOT NULL,
    expected_nerv_minor BIGINT NOT NULL,
    expected_cost_minor BIGINT NOT NULL,
    resource_consumption JSONB NOT NULL,
    allocation_rank INTEGER NOT NULL,
    included BOOLEAN NOT NULL,
    exclusion_reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(run_id,case_id)
);

CREATE TRIGGER promise_events_immutable BEFORE UPDATE OR DELETE ON promise_events FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER merchant_profiles_immutable BEFORE UPDATE OR DELETE ON merchant_optimization_profiles FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER attributions_immutable BEFORE UPDATE OR DELETE ON recovery_attributions FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER feedback_immutable BEFORE UPDATE OR DELETE ON feedback_records FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER datasets_immutable BEFORE UPDATE OR DELETE ON training_dataset_versions FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER model_entries_immutable BEFORE UPDATE OR DELETE ON model_registry_entries FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER model_status_immutable BEFORE UPDATE OR DELETE ON model_registry_status_events FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER priorities_immutable BEFORE UPDATE OR DELETE ON portfolio_priority_snapshots FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER allocation_runs_immutable BEFORE UPDATE OR DELETE ON budget_allocation_runs FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER allocations_immutable BEFORE UPDATE OR DELETE ON budget_allocations FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();

UPDATE platform_metadata SET value='phase_24' WHERE key='schema_version';
