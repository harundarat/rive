# Rive Protocol

> **The settlement layer for AI agents.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](#license--contact)
[![Made with Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Solidity](https://img.shields.io/badge/Solidity-0.8.33-363636?logo=solidity)](https://soliditylang.org)

|                    |                                             |
| ------------------ | ------------------------------------------- |
| 🌐 **Live site**   | https://www.riveprotocol.tech               |
| 🎬 **Demo video**  | https://www.youtube.com/watch?v=Dk71vqM9bo0 |

---

## 1. Problem & Solution

**Problem.** Agent-to-agent payments today are raw ERC-20 transfers. There is no escrow, no double-entry audit trail, and no way to settle high-frequency micropayments between many agents without paying gas on every leg. As soon as you have more than two agents transacting in a tight loop — a common pattern in any non-trivial agent workflow — the per-tx overhead and lack of accountability make on-chain settlement economically and operationally unworkable.

**Solution.** Rive is a settlement layer that sits between agents and the chain, organised around three pillars:

1. **Trustless Escrow.** Funds are locked on-chain in `Escrow.sol` and only released against a cryptographically signed delivery proof of the form `deliver:<orderID>:<deliveryHash>`. Agents sign their own proofs — Rive's backend never holds an agent's private key for escrow operations.
2. **Double-entry Bookkeeping Engine.** Every state change emits a journal entry. Canonical-JSON (RFC 8785) hashes of each entry, each work-order spec, and each settlement manifest are persisted as canonical-JSON records in Postgres. The result is a complete, replayable ledger.
3. **Netting Engine.** Per-window batching compresses N logical payment intents between many agents into a single multi-transfer settlement transaction, executed by `NettingSettlement.settleBatch()` over arrays of net debtors and creditors.

---

## 2. Architecture

```text
                          ┌──────────────────┐
                          │      Agents      │
                          │  (EOA wallets,   │
                          │   sign proofs)   │
                          └────────┬─────────┘
                                   │ HTTP API + signed proofs
                                   ▼
       ┌────────────────────────────────────────────────────────┐
       │              Rive Backend (Go, port :8080)             │
       │   work-orders · netting engine · double-entry ledger   │
       │   ┌──────────────────────────────────────────────────┐ │
       │   │  PostgreSQL — journal, intents, orders, batches, │ │
       │   │              storage_contents                    │ │
       │   └──────────────────────────────────────────────────┘ │
       └────────────────────────────┬───────────────────────────┘
                                    │ settleBatch() /
                                    │ delivery proofs
                                    ▼
                          ┌─────────────────────┐
                          │      EVM Chain      │
                          │  Escrow.sol         │
                          │  NettingSettlement  │
                          │  RiveUSD.sol        │
                          └─────────────────────┘
```

**Stack**

| Layer              | Tech                                                                                                                                          |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Backend            | Go 1.26, chi v5 router, pgx → PostgreSQL. Strict Clean Architecture: `cmd → app → delivery → usecase ← domain ← repository ← infrastructure`. |
| Smart contracts    | Solidity 0.8.33 + Foundry. OpenZeppelin `SafeERC20` + `ReentrancyGuard`.                                                                      |
| Frontend           | Next.js 16 + React 19 + TypeScript + Tailwind 4 (in [`web/`](./web)).                                                                         |
| Off-chain plumbing | QuickNode webhooks → escrow event ingestion (HMAC-SHA256 verified). Goose migrations on Postgres.                                             |

---

## 3. Trust Model

The Rive backend **never** holds an agent's private key for escrow operations.

- **Agents sign their own proofs** — escrow funding (`transferFrom` from the agent's wallet) and delivery proofs (`deliver:<orderID>:<deliveryHash>`, verified via ECDSA recovery).
- **The backend holds exactly one signing key** — `NETTING_SETTLER_PRIVATE_KEY`, used only to call the permissioned `settleBatch()` function.
- **Even `settleBatch` cannot move funds unilaterally** — it uses `transferFrom`, so each debtor agent must have explicitly `approve()`d `NettingSettlement` for the relevant amount before settlement.

---

## 4. API Reference

Every agent wallet must be registered once before it can create work orders or submit payment intents. The demo CLIs (`make demo-escrow` / `make demo-netting`) handle this automatically via direct DB seeding. When integrating directly against the API, call the onboard endpoint first:

```bash
curl -X POST http://localhost:8080/api/agents/onboard \
  -H "Content-Type: application/json" \
  -d '{"wallet_address": "0xYOUR_AGENT_WALLET"}'
# → 201 Created  { "id": "...", "wallet_address": "0x...", ... }
```

Full endpoint reference:

| Method | Endpoint                                     | Description                                                          |
| ------ | -------------------------------------------- | -------------------------------------------------------------------- |
| `POST` | `/api/agents/onboard`                        | **Register an agent wallet** — prerequisite for all write operations |
| `POST` | `/api/work-orders`                           | Create a work order and persist spec to storage                      |
| `GET`  | `/api/work-orders/{onchainOrderID}`          | Fetch work order status                                              |
| `POST` | `/api/work-orders/{onchainOrderID}/delivery` | Submit a signed delivery proof                                       |
| `POST` | `/api/payments/intent`                       | Submit a payment intent (netting flow)                               |
| `GET`  | `/api/ledger/{walletAddress}/pnl`            | Get agent PnL report from the double-entry ledger                    |
| `POST` | `/api/webhooks/quicknode/escrow-events`      | QuickNode webhook for on-chain escrow event ingestion                |
| `GET`  | `/api/health/`                               | Health check                                                         |

---

## 5. Repository Structure

```
rive/
├── contracts/         # Foundry project: Escrow, NettingSettlement, RiveUSD (mock)
├── backend/           # Go REST API + demo CLIs (Clean Architecture)
│   ├── cmd/api/             # HTTP server entrypoint (:8080)
│   ├── cmd/demo-escrow/     # Escrow end-to-end demo CLI
│   ├── cmd/demo-netting/    # Netting end-to-end demo CLI
│   ├── internal/            # app / delivery / usecase / domain / repository / infrastructure
│   └── migrations/          # Goose SQL migrations
├── web/               # Next.js dashboard (PnL report, audit trail, batches)
├── demo/              # YAML configs for the demo CLIs (escrow.*.yaml, netting.*.yaml)
├── docs/              # Protocol walkthroughs (escrow-demo, netting-demo, work-order-lifecycle)
└── Makefile           # demo-escrow / demo-netting targets
```

---

## 6. Quick Start

> **TL;DR** — once `.env` files and `demo/*.local.yaml` are filled in (see steps 2-3):
>
> ```bash
> make demo-escrow     # 1 escrow lifecycle end-to-end
> make demo-netting    # 20 intents → 1 settleBatch() tx
> ```
>
> Full setup details below.

### Prerequisites

- **Go** 1.26+
- **Node** 20+ and **pnpm** 10+
- **Foundry** (`forge`, `cast`)
- **PostgreSQL** 14+ (local or Dockerised)
- A deployed instance of `Escrow.sol` and `NettingSettlement.sol` on an EVM-compatible chain

### 1. Clone & install

```bash
git clone https://github.com/harundarat/rive.git
cd rive

cd backend && go mod download && cd ..
cd web && pnpm install && cd ..
cd contracts && forge install && cd ..
```

### 2. Configure environment

```bash
cp backend/.env.example backend/.env
cp contracts/.env.example contracts/.env
```

Key variables to fill in `backend/.env`:

| Variable                      | Purpose                                                    |
| ----------------------------- | ---------------------------------------------------------- |
| `NETTING_EVM_RPC`             | EVM RPC endpoint for the settlement chain                  |
| `DB_*`                        | Postgres connection (host / port / user / password / name) |
| `ESCROW_CONTRACT_ADDRESS`     | Deployed address of `Escrow.sol`                           |
| `NETTING_SETTLEMENT_ADDRESS`  | Deployed address of `NettingSettlement.sol`                |
| `NETTING_SETTLER_PRIVATE_KEY` | Backend's settler EOA (only key the backend holds)         |
| `NETTING_WINDOW_SECONDS`      | Batch close interval (5 for demo, 60 default)              |
| `QUICKNODE_WEBHOOK_SECRET`    | HMAC secret for webhook verification                       |
| `CORS_ALLOWED_ORIGINS`        | Comma-separated origins for the dashboard                  |

### 3. Create local demo configs

```bash
cp demo/escrow.example.yaml  demo/escrow.local.yaml
cp demo/netting.example.yaml demo/netting.local.yaml
```

Fill in `private_key` for each agent.

> **Agent registration:** The demo CLIs automatically register each configured wallet into the database at startup — no manual step required. If you are calling the API directly (outside of the demo CLIs), register each agent wallet first via `POST /api/agents/onboard` before creating work orders or submitting payment intents (see [API Reference](#4-api-reference) above).

### 4. Migrate database & run backend

```bash
# Apply migrations (goose)
cd backend
goose -dir migrations postgres "$DATABASE_URL" up

# Start API server on :8080
go run ./cmd/api
```

### 5. Start the dashboard

```bash
cd web
pnpm dev    # http://localhost:3000
```

### 6. Run the demos

From the repo root:

```bash
make demo-escrow     # 1 escrow lifecycle: Buyer → Processor
make demo-netting    # 5 agents, 20 intents → 1 settlement tx
```

---

## 7. Demo Flow

### Escrow demo — `make demo-escrow`

Two agents: a **Data Buyer** and a **Data Processor**.

1. Buyer creates a work order; spec is persisted to storage and `specHash` committed on-chain.
2. Buyer funds the order on `Escrow.sol` (rUSD locked, `transferFrom` from buyer's wallet).
3. Processor signs `deliver:<orderID>:<deliveryHash>` and submits the proof.
4. Escrow verifies the signature and releases payment to the processor.
5. Backend writes a double-entry journal entry per state change, persisted in Postgres.

### Netting demo — `make demo-netting`

Five agents: `scout`, `analyst`, `data`, `verifier`, `router`. They submit **20 cross-paying payment intents** to `POST /api/payments/intent`.

```text
Stage 1 — Off-chain netting:
  20 logical intents  →  5 net positions (one per agent, debit or credit)

Stage 2 — On-chain settlement:
  5 net positions  →  1 settleBatch() transaction
                      (Solidity for-loop over debtor and creditor arrays)

Result:
  Gross volume:               79 rUSD across 20 intents
  Net settlement amount:      23 rUSD moved on-chain
  On-chain transactions:      1 (vs 20 individual transfers without netting)
```

The CLI polls Postgres until the batch is settled and prints the settlement tx hash.

---

## 8. Roadmap

**V2**

- Real USDC settlement (replaces rUSD mock).
- On-chain dispute resolution module.
- TEE-attested delivery verification (replace signature-only proofs).

**V3**

- Multi-asset settlement (any ERC-20 / native).
- Cross-chain settlement with bridge integrations.
- Agent-side SDK in Go and TypeScript.

---

## 9. Contact

Built by **Harun** ([@harundarat](https://github.com/harundarat)).
