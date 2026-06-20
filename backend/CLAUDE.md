# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Scope

This file covers the Go backend in `backend/`. The repo also contains `web/` (Next.js frontend, has its own CLAUDE.md) and `contracts/` (Solidity). The backend talks to those via HTTP and on-chain calls but does not import them.

## Commands

Run all from `backend/` (this directory). Go module path: `github.com/harundarat/rive/backend`.

- Run the API server: `go run ./cmd/api` (listens on `:8080`, reads `.env` if present, otherwise uses system env vars)
- Build: `go build ./...`
- Vet: `go vet ./...`
- All tests: `go test ./...`
- Single package: `go test ./internal/usecase`
- Single test: `go test ./internal/usecase -run TestNettingUsecase_SubmitIntent`
- Verbose / race: `go test -race -v ./...`

Migrations live in `migrations/` and use `pressly/goose` syntax (`-- +goose Up` / `-- +goose Down`). They are NOT auto-applied — apply with the `goose` CLI against `DATABASE` configured via `DB_*` env vars.

Demo CLIs (end-to-end smoke flows that exercise a running API + on-chain contracts; not unit tests):
- `go run ./cmd/demo-escrow -config ../demo/escrow.local.yaml`
- `go run ./cmd/demo-netting -config ../demo/netting.local.yaml`

## Configuration

`config.Load()` loads `.env` from the **current working directory** if present, then falls back to system environment variables (each key is bound via `viper.BindEnv`). The `.env` file is optional — required keys can come from either source, which makes deployment to containerized/PaaS environments straightforward. A malformed `.env` still fails startup (fail-fast), only "file not found" is tolerated. See `.env.example` for the full set; required groups:
- `DB_*` — Postgres / CockroachDB connection. SSL is on by default (`DB_SSLMODE=verify-full` + `DB_SSLROOTCERT=./ca.pem`). Storage of canonical-JSON payloads also lives here (`storage_contents` table).
- `QUICKNODE_WEBHOOK_SECRET`, `ESCROW_CONTRACT_ADDRESS` — incoming on-chain event webhook.
- `NETTING_EVM_RPC`, `NETTING_SETTLEMENT_ADDRESS`, `NETTING_SETTLER_PRIVATE_KEY`, `NETTING_WINDOW_SECONDS` — netting batch settler (EVM RPC + settlement contract). **If the RPC, address, or private key is empty, `NettingGateway` runs in `disabled` mode (logs but does not submit on-chain tx).** This is intentional for local dev and tests.

Note: the netting window env var is `NETTING_WINDOW_SECONDS` (plural; config field `WindowSeconds`, default 60). `.env.example` matches this spelling.

## Architecture

Strict Clean Architecture with one-way dependency flow: `cmd → app → delivery → usecase → domain ← repository ← infrastructure`. The `doc.go` files in each layer state the rules — **`internal/domain` must not import any other layer**, and usecases speak only to interfaces declared there.

Layers:
- `cmd/api/main.go` — process entrypoint; calls `app.Initialize()` and starts `net/http` on `:8080`.
- `internal/app/app.go` — composition root. Wires DB pool, Postgres-backed storage, repositories, usecases, handlers, and the chi router. **All DI happens here** — when adding a new feature, register it in `Initialize()` and pass it down to `NewRouter`.
- `internal/delivery/http` — chi handlers + `router.go`. Responses use `pkg/response.Envelope[T]` (`{success, data, error, meta}`); errors use `pkg/apierror.APIError`. Always go through these, not raw `json.NewEncoder`.
- `internal/usecase` — business logic. One usecase per aggregate (work order, pnl, netting, health). Long-lived background loops (e.g. `nettingUsecase.Start(ctx)`) are kicked off in `app.Initialize`.
- `internal/domain` — entities + repository/usecase interfaces + sentinel errors (`ErrNotFound`, `ErrPersistence`, validation errors via `NewValidationError`). Never put framework types here.
- `internal/repository/postgres` — pgx-backed implementations of the domain repository interfaces.
- `internal/infrastructure` — connection setup: `database/postgres.go` (pgxpool), `storage/dbstorage.go` (Postgres-backed storage client), `settlement/netting.go` (eth client + ABI-bound settler).
- `pkg/` — small reusable utilities reachable by anyone: `apierror`, `response`, `canonicaljson` (RFC 8785 / JCS for deterministic hashing of payloads written to storage).

### Cross-cutting concerns to know about

- **Idempotency**: work orders and payment intents are deduplicated by `idempotency_key`. Repositories expose `FindByIdempotencyKey`; usecases short-circuit and return the existing record on a hit. Preserve this pattern when adding write endpoints.
- **On-chain event ingestion**: `QuickNodeWebhookHandler` (POST `/api/webhooks/quicknode/escrow-events`) verifies an HMAC-SHA256 signature against `QUICKNODE_WEBHOOK_SECRET`, filters logs by `ESCROW_CONTRACT_ADDRESS`, and decodes the three event topics (`OrderCreated`, `OrderReleased`, `OrderRefunded`) defined as constants in `quicknode_webhook_handler.go`. Each event has paired `Record*` / `Rollback*` methods on `WorkOrderOnchainEventUsecase` to handle reorgs.
- **Netting settlement**: `NettingUsecase.Start(ctx)` runs a periodic loop (`NETTING_WINDOW_SECONDS`) that closes the open batch, computes debtor/creditor positions, writes a canonical-JSON manifest to Postgres storage, and calls `settleBatch(bytes32, address[], uint256[], address[], uint256[])` on the settlement contract. If `NettingGateway` is disabled (missing config), the on-chain step is skipped but DB state still advances — useful for tests, dangerous in prod.
- **Money types**: amounts are `math/big.Int` end-to-end (DB → domain → on-chain). Don't round-trip through `int64` or `float64`.
- **Canonical JSON**: anything written to storage or hashed for on-chain commitment goes through `pkg/canonicaljson` — do not `json.Marshal` directly for those payloads.

## Testing notes

- Unit tests use hand-written fakes/stubs (e.g. `fakeNettingTx` implementing pgx's `Tx`), not `testcontainers` or sqlmock — match this style when adding tests rather than introducing a new mocking framework.
- `internal/repository/postgres/*_test.go` test SQL via the same fake-pgx pattern; they assert on the SQL strings and arg shapes, so be careful when reformatting queries.
- The `cmd/demo-*` programs are not run by `go test ./...` against real infra automatically — only their `config_test.go` files run as units.

## Conventions

- Errors bubble up wrapped with `%w` and a short context prefix (`"find existing payment intent: %w"`). At the HTTP layer they're translated to `apierror.APIError` via the sentinel set in `pkg/apierror`.
- Logging is via `log.Printf` / `logrus` (mixed). New code should prefer the package's existing choice rather than introducing a third logger.
- UUIDs use v7 (`uuid.NewV7`) for time-ordered IDs. Don't switch to v4 without reason.
