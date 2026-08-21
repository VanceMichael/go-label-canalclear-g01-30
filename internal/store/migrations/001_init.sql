CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL,
    disabled boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id),
    tenant_id text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS sessions_user_active_idx ON sessions(user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS voyages (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    vessel_imo text NOT NULL,
    vessel_name text NOT NULL,
    origin_port text NOT NULL,
    destination_port text NOT NULL,
    eta timestamptz NOT NULL,
    status text NOT NULL,
    manifest_hash text NOT NULL DEFAULT '',
    declared_at timestamptz,
    cleared_at timestamptz,
    version bigint NOT NULL
);
CREATE INDEX IF NOT EXISTS voyages_tenant_status_idx ON voyages(tenant_id, status, eta);

CREATE TABLE IF NOT EXISTS manifests (
    voyage_id text PRIMARY KEY REFERENCES voyages(id) ON DELETE CASCADE,
    hash text NOT NULL,
    items jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS inspections (
    id text PRIMARY KEY,
    voyage_id text NOT NULL REFERENCES voyages(id),
    officer_id text NOT NULL,
    kind text NOT NULL,
    status text NOT NULL,
    finding text NOT NULL DEFAULT '',
    opened_at timestamptz NOT NULL,
    closed_at timestamptz,
    version bigint NOT NULL
);

CREATE TABLE IF NOT EXISTS passage_reservations (
    id text PRIMARY KEY,
    voyage_id text NOT NULL REFERENCES voyages(id),
    tenant_id text NOT NULL,
    chamber_id text NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    length_m double precision NOT NULL,
    beam_m double precision NOT NULL,
    draft_m double precision NOT NULL,
    status text NOT NULL,
    version bigint NOT NULL,
    EXCLUDE USING gist (chamber_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (status <> 'cancelled')
);

CREATE TABLE IF NOT EXISTS audit_events (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    action text NOT NULL,
    object_type text NOT NULL,
    object_id text NOT NULL,
    outcome text NOT NULL,
    details jsonb NOT NULL,
    occurred_at timestamptz NOT NULL,
    sequence bigint NOT NULL,
    previous_hash text NOT NULL,
    hash text NOT NULL UNIQUE,
    UNIQUE (tenant_id, sequence)
);

CREATE TABLE IF NOT EXISTS outbox_jobs (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    topic text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL,
    locked_at timestamptz,
    locked_by text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS outbox_ready_idx ON outbox_jobs(status, available_at);
