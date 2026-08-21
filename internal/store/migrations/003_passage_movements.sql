CREATE TABLE IF NOT EXISTS passage_movements (
    sequence bigserial PRIMARY KEY,
    reservation_id text NOT NULL REFERENCES passage_reservations(id),
    status text NOT NULL,
    occurred_at timestamptz NOT NULL,
    operator_id text NOT NULL,
    note text NOT NULL DEFAULT '',
    UNIQUE (reservation_id, status),
    CHECK (status IN ('entered', 'exited', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS passage_movements_reservation_idx
    ON passage_movements (reservation_id, occurred_at, sequence);
