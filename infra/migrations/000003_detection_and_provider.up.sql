ALTER TABLE webhook_events
    ADD COLUMN signature_status TEXT NOT NULL DEFAULT 'UNVERIFIED'
        CHECK (signature_status IN ('VERIFIED','INVALID','UNVERIFIED')),
    ADD COLUMN provider_references JSONB NOT NULL DEFAULT '{}';

CREATE TABLE checkout_sessions (
    checkout_id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants(id),
    customer_id TEXT NOT NULL REFERENCES customers(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency CHAR(3) NOT NULL,
    stage TEXT NOT NULL,
    payment_method TEXT NOT NULL DEFAULT '',
    valid_until TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE provider_action_references (
    id TEXT PRIMARY KEY,
    action_id TEXT NOT NULL REFERENCES recovery_actions(id),
    provider TEXT NOT NULL,
    operation TEXT NOT NULL,
    provider_reference TEXT NOT NULL,
    response JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (action_id, provider, operation)
);

UPDATE platform_metadata SET value='phase_10' WHERE key='schema_version';
