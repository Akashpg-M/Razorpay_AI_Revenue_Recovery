ALTER TABLE recovery_decisions
    DROP COLUMN IF EXISTS eligibility_snapshot,
    DROP COLUMN IF EXISTS decision_context;

UPDATE platform_metadata SET value='phase_34' WHERE key='schema_version';
