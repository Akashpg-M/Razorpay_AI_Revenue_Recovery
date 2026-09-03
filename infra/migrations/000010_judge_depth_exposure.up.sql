ALTER TABLE recovery_decisions
    ADD COLUMN decision_context JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN eligibility_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE platform_metadata SET value='phase_55' WHERE key='schema_version';
