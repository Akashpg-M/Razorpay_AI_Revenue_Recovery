DROP INDEX IF EXISTS executions_idempotency_idx;
CREATE UNIQUE INDEX executions_idempotency_idx ON executions(idempotency_key);
UPDATE platform_metadata SET value='phase_16.1' WHERE key='schema_version';
