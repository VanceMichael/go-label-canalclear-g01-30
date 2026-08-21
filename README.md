# CanalClear

CanalClear is a PostgreSQL-backed canal-port clearance and lock-passage coordination service. Carrier operators declare voyages and canonical manifests, customs officers open and close inspections, dispatchers reserve lock chambers only after release, and every connected workflow writes a tamper-evident tenant audit chain and reliable outbox event.

## Run

```bash
docker compose up --build
```

Health endpoints are `/healthz` and `/readyz`. Demo accounts use password `canalclear-demo-password`:

- `carrier@canalclear.test`
- `dispatcher@canalclear.test`
- `customs@canalclear.test`
- `lock@canalclear.test`
- `auditor@canalclear.test`

The service exposes login/logout, voyage declaration and lookup, customs inspection and release, lock passage reservation, tenant-scoped voyage search, and audit event search APIs under `/v1`.

## Verification

Unit and contract tests do not require online services. PostgreSQL integration tests use the same database semantics as production:

```bash
go test ./... -count=1
go test -race ./... -count=1
TEST_DATABASE_URL='postgres://canalclear:canalclear@127.0.0.1:55441/canalclear?sslmode=disable' go test ./... -count=1
go vet ./...
go build ./...
```

The database migrations create users, revocable sessions, voyages, canonical manifests, inspections, passage reservations and movements, a tenant audit hash chain, idempotency records, and an outbox. Workflow writes use serializable transactions, optimistic versions, PostgreSQL exclusion constraints for chamber collisions, and tenant-scoped advisory locks for audit ordering.

The outbox worker claims jobs with `SKIP LOCKED`, retries with bounded exponential backoff, records permanent failure, and recovers expired worker locks. Request middleware provides request IDs, structured access logs, panic recovery, security headers, and a stable JSON error contract.
