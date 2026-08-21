CREATE TABLE IF NOT EXISTS idempotency_records (
    tenant_id text NOT NULL,
    key text NOT NULL,
    operation text NOT NULL,
    request_hash text NOT NULL,
    status text NOT NULL,
    response_code integer NOT NULL DEFAULT 0,
    response_body bytea NOT NULL DEFAULT '',
    failure_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    version bigint NOT NULL,
    PRIMARY KEY (tenant_id, key),
    CHECK (status IN ('processing', 'completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idempotency_expiry_idx
    ON idempotency_records (expires_at);
