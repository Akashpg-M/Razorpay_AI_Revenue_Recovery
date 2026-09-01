ALTER TABLE attribution_rule_configs
    ADD COLUMN precedence JSONB NOT NULL DEFAULT '["PTP","RETRY","DIRECT_ACTION","NATURAL","UNKNOWN"]';

INSERT INTO attribution_rule_configs(version,retry_window_minutes,direct_action_window_minutes,email_assist_window_minutes,ptp_window_minutes,precedence,created_at)
VALUES('attribution-v2',1440,10080,4320,1440,'["EXACT_PROVIDER_REFERENCE","PTP","RETRY","DIRECT_ACTION","NATURAL","UNKNOWN"]',NOW());

CREATE TABLE resilience_evaluation_runs (
    id TEXT PRIMARY KEY,
    suite TEXT NOT NULL CHECK(suite IN('AUTHORIZATION_SAFETY','FAULT_RECONCILIATION','INTERACTIVE')),
    environment TEXT NOT NULL,
    scenario TEXT NOT NULL,
    fault_mode TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    provider_effect_count INTEGER NOT NULL CHECK(provider_effect_count >= 0),
    execution_attempt_count INTEGER NOT NULL CHECK(execution_attempt_count >= 0),
    result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX resilience_runs_created_idx ON resilience_evaluation_runs(created_at DESC);
CREATE TRIGGER resilience_runs_immutable BEFORE UPDATE OR DELETE ON resilience_evaluation_runs FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();

UPDATE platform_metadata SET value='phase_30' WHERE key='schema_version';
