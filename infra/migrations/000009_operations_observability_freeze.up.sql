CREATE TABLE human_review_records (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES recovery_cases(id),
    decision_id TEXT NOT NULL REFERENCES recovery_decisions(id),
    policy_evaluation_id TEXT NOT NULL REFERENCES policy_evaluations(id),
    recommended_action TEXT NOT NULL,
    operator_id TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK(actor_type IN('OPERATOR','SUPERVISOR','SYSTEM_TEST')),
    actor_metadata JSONB NOT NULL DEFAULT '{}',
    decision TEXT NOT NULL CHECK(decision IN('APPROVE','REJECT','DEFER','STOP')),
    reason_code TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    case_version_at_review BIGINT NOT NULL,
    merchant_policy_version_at_review INTEGER NOT NULL,
    review_after TIMESTAMPTZ,
    idempotency_key TEXT NOT NULL UNIQUE,
    reauthorization_result TEXT NOT NULL CHECK(reauthorization_result IN('APPROVED','NOT_REQUIRED','STALE_APPROVAL','DENIED','STOPPED')),
    reauthorization_reason_codes TEXT[] NOT NULL DEFAULT '{}',
    scheduled_action_id TEXT REFERENCES scheduled_actions(id),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX human_reviews_case_time_idx ON human_review_records(case_id,created_at DESC,id DESC);
CREATE INDEX human_reviews_deferred_idx ON human_review_records(review_after) WHERE decision='DEFER';
CREATE TRIGGER human_reviews_immutable BEFORE UPDATE OR DELETE ON human_review_records FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();

UPDATE platform_metadata SET value='phase_34' WHERE key='schema_version';
