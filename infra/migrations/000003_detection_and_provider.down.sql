DROP TABLE IF EXISTS provider_action_references;
DROP TABLE IF EXISTS checkout_sessions;
ALTER TABLE webhook_events DROP COLUMN IF EXISTS provider_references;
ALTER TABLE webhook_events DROP COLUMN IF EXISTS signature_status;
UPDATE platform_metadata SET value='phase_4' WHERE key='schema_version';
