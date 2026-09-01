DROP TABLE IF EXISTS resilience_evaluation_runs;
ALTER TABLE attribution_rule_configs DROP COLUMN IF EXISTS precedence;
UPDATE platform_metadata SET value='phase_24' WHERE key='schema_version';
