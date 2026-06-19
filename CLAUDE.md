# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Rive is a settlement layer for AI-agent-to-agent payments, built for the 0G APAC Hackathon (Track 3). It is a monorepo of three independently-toolchained subprojects that communicate only over HTTP and on-chain calls — none imports another:

- **`backend/`** — Go 1.26 REST API + demo CLIs (Clean Architecture). Has its own detailed `backend/CLAUDE.md` — **read it before touching Go code.**
- **`web/`** — Next.js 16 / React 19 / Tailwind 4 dashboard. See `web/AGENTS.md`: this is a breaking-changes Next.js version, so consult `web/node_modules/next/dist/docs/` before writing frontend code rather than relying on prior Next.js knowledge.
- **`contracts/`** — Foundry / Solidity 0.8.33: `Escrow.sol`, `NettingSettlement.sol`, `RiveUSD.sol` (mock stablecoin).

The README has the deployed mainnet addresses, the full API reference, and reviewer setup steps.

## Commands

**Root** (`Makefile` — end-to-end demos against a running API + on-chain contracts; require `demo/*.local.yaml` filled in):
- `make demo-escrow` — one escrow lifecycle (Buyer → Processor)
- `make demo-netting` — 5 agents, 20 intents → 1 `settleBatch()` tx

**Backend** (run from `backend/`): `go run ./cmd/api` (serves `:8080`), `go build ./...`, `go test ./...`, single test `go test ./internal/usecase -run TestName`. Full details + DB/migration/config notes in `backend/CLAUDE.md`.

**Web** (run from `web/`, uses **pnpm**): `pnpm dev`, `pnpm build`, `pnpm lint`.

**Contracts** (run from `contracts/`): `forge build`, `forge test`, `forge fmt`. Deploy with `FOUNDRY_PROFILE=0g_mainnet forge script script/Deploy.s.sol:DeployScript --broadcast` (verification + per-contract deploy scripts documented in `contracts/README.md`). OpenZeppelin and forge-std are git submodules under `contracts/lib/` — run `forge install` / `git submodule update --init` after cloning.

## Cross-component architecture

The whole system exists to make agent payments **cryptographically auditable on-chain**. The invariant that ties the three subprojects together: every order spec, journal entry, and netting manifest is serialized as **canonical JSON (RFC 8785 / JCS)**, hashed, pinned to **0G Storage**, and that hash is committed **on-chain**. An external auditor can reconstruct any agent's full ledger from `chain ∪ 0G Storage` alone — so anything written to 0G or hashed for an on-chain commitment must go through `backend/pkg/canonicaljson`, never a raw `json.Marshal`.

Two flows span backend ↔ contracts:

1. **Escrow** (`Escrow.sol`): an order's `Created` state *is* funded — creation and the `transferFrom` are atomic; the contract anchors only `specHash` (spec lives off-chain in 0G). Release requires the payee's signed delivery proof `deliver:<orderID>:<deliveryHash>` (ECDSA-recovered on-chain). After `REFUND_TIMEOUT` (7 days) anyone may refund the payer. The backend ingests `OrderCreated`/`OrderReleased`/`OrderRefunded` via the HMAC-verified QuickNode webhook.

2. **Netting** (`NettingSettlement.sol`): the backend batches many off-chain payment intents into one `settleBatch(batchHash, debtors[], debitAmounts[], creditors[], creditAmounts[])` call. The contract enforces: caller must be the `settler` EOA, `debitTotal == creditTotal`, and each `batchHash` settles at most once. It uses `safeTransferFrom`, so **each debtor must have `approve()`d the contract beforehand** — settlement cannot move funds unilaterally.

**Trust model:** the backend holds exactly one key, `NETTING_SETTLER_PRIVATE_KEY`, used only for `settleBatch()`. Agents sign their own escrow funding and delivery proofs; the backend never holds an agent's escrow key.

**Frontend ↔ backend:** the dashboard is read-only over the API (e.g. `GET /api/ledger/{wallet}/pnl`). Base URL is `NEXT_PUBLIC_RIVE_API_BASE_URL` (defaults to `http://localhost:8080`); the backend's `CORS_ALLOWED_ORIGINS` must include the dashboard origin.

## Conventions worth knowing repo-wide

- **Money is `big.Int` end-to-end** in Go (DB → domain → on-chain). Never round-trip amounts through `int64`/`float64`.
- **Idempotency:** write endpoints (work orders, payment intents) dedupe on an `idempotency_key`; preserve this when adding write paths.
- The two `cmd/demo-*` CLIs auto-register their configured agent wallets at startup. When integrating against the API directly, call `POST /api/agents/onboard` first.
