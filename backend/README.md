# rive — backend

Go REST API for the Rive settlement layer. Handles escrow work orders, netting batches, agent onboarding, and on-chain event ingestion.

## Tech Stack

### Core
- Go 1.26 (standard library)
- [pgx/v5](https://github.com/jackc/pgx) — PostgreSQL driver
- [go-chi/chi/v5](https://github.com/go-chi/chi) — HTTP router
- [google/uuid](https://github.com/google/uuid) — v7 time-ordered IDs
- [gowebpki/jcs](https://github.com/gowebpki/jcs) — canonical JSON (RFC 8785 / JCS)
- [spf13/viper](https://github.com/spf13/viper) — configuration

### Infrastructure
- CockroachDB (CockroachDB Cloud Free Tier) — primary database + canonical-JSON storage
- EVM RPC (Alchemy / QuickNode) — on-chain settlement

### Web3
- [go-ethereum](https://github.com/ethereum/go-ethereum)

## Commands

Run all commands from `backend/`.

```sh
# Start API server (listens on :8080)
go run ./cmd/api

# Build
go build ./...

# Vet
go vet ./...

# All tests
go test ./...

# Single package / single test
go test ./internal/usecase
go test ./internal/usecase -run TestNettingUsecase_SubmitIntent

# With race detector
go test -race -v ./...
```

**Demo CLIs** — end-to-end smoke flows against a running API + on-chain contracts:

```sh
go run ./cmd/demo-escrow -config ../demo/escrow.local.yaml
go run ./cmd/demo-netting -config ../demo/netting.local.yaml
```

**Migrations** — apply with the `goose` CLI before first run:

```sh
goose -dir migrations postgres "$DSN" up
```

Migrations use `-- +goose Up` / `-- +goose Down` syntax and are **not** auto-applied at startup.

## Configuration

Copy `.env.example` to `.env` in `backend/`. The server reads `.env` from the working directory on startup and falls back to system environment variables. A missing file is tolerated; a malformed one fails fast.

| Variable | Description |
|----------|-------------|
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | Database connection |
| `DB_SSLMODE` | SSL mode (default `verify-full`) |
| `DB_SSLROOTCERT` | Path to CA PEM file (local dev) |
| `DB_SSL_CA` | PEM content inline (PaaS / Railway alternative) |
| `DB_MAX_IDLE_CONN`, `DB_MAX_OPEN_CONN` | Connection pool sizing |
| `QUICKNODE_WEBHOOK_SECRET` | HMAC-SHA256 key for webhook verification |
| `ESCROW_CONTRACT_ADDRESS` | Filter escrow event logs by contract |
| `NETTING_EVM_RPC` | EVM JSON-RPC endpoint |
| `NETTING_SETTLEMENT_ADDRESS` | `NettingSettlement` contract address |
| `NETTING_SETTLER_PRIVATE_KEY` | EOA key used to sign `settleBatch()` calls |
| `NETTING_WINDOW_SECONDS` | Netting batch interval in seconds (default `60`) |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed origins for the dashboard |

If `NETTING_EVM_RPC`, `NETTING_SETTLEMENT_ADDRESS`, or `NETTING_SETTLER_PRIVATE_KEY` is empty, `NettingGateway` runs in **disabled** mode — the periodic loop still closes batches and writes to the DB, but no on-chain transaction is submitted. This is the correct default for local dev and unit tests.

## API Endpoints

All responses use the envelope shape `{"success": bool, "data": ..., "error": ..., "meta": ...}`.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/agents/onboard` | Register an agent wallet |
| `POST` | `/api/work-orders` | Create a work order (serializes spec as canonical JSON to storage) |
| `GET` | `/api/work-orders/{onchainOrderID}` | Fetch a work order by on-chain order ID |
| `POST` | `/api/work-orders/{onchainOrderID}/delivery` | Submit a signed delivery proof |
| `GET` | `/api/ledger/{walletAddress}/pnl` | P&L for a wallet (`?from=` / `?to=` optional ISO-8601 range) |
| `POST` | `/api/payments/intent` | Submit a payment intent for the netting batch |
| `POST` | `/api/storage/upload` | Upload raw bytes or JSON to Postgres-backed storage |
| `POST` | `/api/webhooks/quicknode/escrow-events` | QuickNode escrow event webhook (HMAC-SHA256 verified) |
| `GET` | `/api/health/` | Liveness check |
| `POST` | `/api/health/storage` | Storage write smoke-test |

## Architecture

Strict Clean Architecture with a one-way dependency rule: `cmd → app → delivery/http → usecase → domain ← repository ← infrastructure`.

| Layer | Package | Role |
|-------|---------|------|
| Entrypoint | `cmd/api` | Process start; calls `app.Initialize()` and starts `net/http` |
| Composition root | `internal/app` | Wires DB pool, repositories, usecases, handlers, and router |
| Delivery | `internal/delivery/http` | chi handlers + `router.go`; uses `pkg/response` and `pkg/apierror` |
| Usecase | `internal/usecase` | Business logic; background netting loop started here |
| Domain | `internal/domain` | Entities, repository/usecase interfaces, sentinel errors |
| Repository | `internal/repository/postgres` | pgx-backed implementations |
| Infrastructure | `internal/infrastructure` | DB pool, Postgres storage client, EVM settlement gateway |
| Utilities | `pkg/` | `apierror`, `response`, `canonicaljson` |

See [`CLAUDE.md`](./CLAUDE.md) for full architectural detail, cross-cutting concerns (idempotency, money types, canonical JSON), and testing conventions.

## Database Migrations

Eight migrations in `migrations/`, applied in order:

1. `00001_agents.sql`
2. `00002_accounts.sql`
3. `00003_netting_batches.sql`
4. `00004_payment_intents.sql`
5. `00005_work_orders.sql`
6. `00006_journal_entries.sql`
7. `00007_ledger_entries.sql`
8. `00008_storage_contents.sql`
