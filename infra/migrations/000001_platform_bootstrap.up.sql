CREATE TABLE IF NOT EXISTS platform_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO platform_metadata (key,value)
VALUES ('schema_version','phase_1')
ON CONFLICT (key)
DO UPDATE SET
    value = EXCLUDED.value;