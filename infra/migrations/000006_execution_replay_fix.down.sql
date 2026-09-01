DROP INDEX IF EXISTS executions_idempotency_idx;
CREATE UNIQUE INDEX executions_idempotency_idx ON executions(idempotency_key) WHERE idempotency_key IS NOT NULL;
UPDATE platform_metadata SET value='phase_16' WHERE key='schema_version';
