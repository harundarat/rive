# Rive Protocol

> **The settlement layer for AI agents.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](#license--contact)
[![Hackathon](https://img.shields.io/badge/0G_APAC_Hackathon-2026_·_Track_3-7c3aed)](https://0g.ai)
[![Network](https://img.shields.io/badge/0G_Mainnet-Chain_ID_16661-0ea5e9)](https://chainscan.0g.ai)
[![Made with Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Solidity](https://img.shields.io/badge/Solidity-0.8.33-363636?logo=solidity)](https://soliditylang.org)

| | |
| --- | --- |
| 🌐 **Live site** | <!-- TODO: https://riveprotocol.tech --> |
| 🎬 **Demo video** | <!-- TODO: video URL --> |
| 🔍 **0G Explorer** | https://chainscan.0g.ai |

---

## 1. Problem & Solution

**Problem.** Agent-to-agent payments today are raw ERC-20 transfers. There is no escrow, no double-entry audit trail, and no way to settle high-frequency micropayments between many agents without paying gas on every leg. As soon as you have more than two agents transacting in a tight loop — a common pattern in any non-trivial agent workflow — the per-tx overhead and lack of accountability make on-chain settlement economically and operationally unworkable.

**Solution.** Rive is a settlement layer that sits between agents and the chain, organised around three pillars:

1. **Trustless Escrow.** Funds are locked on-chain in `Escrow.sol` and only released against a cryptographically signed delivery proof of the form `deliver:<orderID>:<deliveryHash>`. Agents sign their own proofs — Rive's backend never holds an agent's private key for escrow operations.
2. **Double-entry Bookkeeping Engine.** Every state change emits a journal entry. Canonical-JSON (RFC 8785) hashes of each entry, each work-order spec, and each settlement manifest are pinned to 0G Storage. The result is a complete, replayable ledger keyed by Merkle root.
3. **Netting Engine.** Per-window batching compresses N logical payment intents between many agents into a single multi-transfer settlement transaction, executed by `NettingSettlement.settleBatch()` over arrays of net debtors and creditors.

**Moat.** Cryptographically auditable accounting, on-chain. Because every order, journal entry, and netting batch manifest is committed to 0G Storage by Merkle root and referenced from the chain, an external auditor can reconstruct any agent's full ledger from `chain ∪ 0G Storage` alone — no trust in Rive's backend required.

---

## 2. 0G Integration

| 0G Component | How Rive uses it |
| --- | --- |
| **0G Chain** | Hosts `Escrow.sol`, `NettingSettlement.sol`, and `RiveUSD.sol` (test stable). The settler EOA submits batched netting transactions; agents sign their own escrow funding and delivery proofs. |
| **0G Storage** | Stores work-order specs, per-event escrow journal entries, and netting batch manifests as canonical JSON (RFC 8785). The Merkle root (`specHash`) is pinned on-chain. Client: [`backend/internal/infrastructure/storage/zerog.go`](./backend/internal/infrastructure/storage/zerog.go) using `github.com/0gfoundation/0g-storage-client`. |
| **0G Agent ID** *(lightweight V1)* | Each agent has a stable `agent_id_0g` string (e.g. `rive-demo-scout`) plus an EVM signing address, persisted in the Postgres `agents` table and mirrored on-chain via the agent registry. ERC-7857 iNFT integration is planned for V2. |

### Deployed contracts — 0G Mainnet (Chain ID 16661)

| Contract | Address | Explorer |
| --- | --- | --- |
| RiveUSD (rUSD) | `0xB053E106D5236e4c4cD1b7DA0aC51bA0B318C7a0` | [chainscan.0g.ai](https://chainscan.0g.ai/address/0xB053E106D5236e4c4cD1b7DA0aC51bA0B318C7a0) |
| Escrow | `0xe3de5a57b960aeaa4d1d01b46665599067476b6d` | [chainscan.0g.ai](https://chainscan.0g.ai/address/0xe3de5a57b960aeaa4d1d01b46665599067476b6d) |
| NettingSettlement | `0x59Ecf1AD6e755CBE71aac6DfAf1b2Dba9E148b98` | [chainscan.0g.ai](https://chainscan.0g.ai/address/0x59Ecf1AD6e755CBE71aac6DfAf1b2Dba9E148b98) |

RPC endpoint: `https://evmrpc.0g.ai` · Storage indexer: configured via `ZG_STORAGE_INDEXER_RPC`.

---

## 3. Architecture

<!-- TODO: render and commit ./docs/architecture.png -->
![Rive Protocol architecture](./docs/architecture.png)

**Stack**

| Layer | Tech |
| --- | --- |
| Backend | Go 1.26, chi v5 router, pgx → PostgreSQL. Strict Clean Architecture: `cmd → app → delivery → usecase ← domain ← repository ← infrastructure`. |
| Smart contracts | Solidity 0.8.33 + Foundry. OpenZeppelin `SafeERC20` + `ReentrancyGuard`. |
| Frontend | Next.js 16 + React 19 + TypeScript + Tailwind 4 (in [`web/`](./web)). |
| Off-chain plumbing | QuickNode webhooks → escrow event ingestion (HMAC-SHA256 verified). Goose migrations on Postgres. |

**Trust model.** The backend never holds an agent's private key. Agents sign their own escrow funding and delivery proofs (`deliver:<orderID>:<deliveryHash>`, ECDSA recovery). The backend holds exactly one signing key — `NETTING_SETTLER_PRIVATE_KEY` — used to call the permissioned `settleBatch()` function. Even that call cannot move funds unilaterally: `settleBatch` uses `transferFrom`, so each debtor agent must have explicitly approved `NettingSettlement` for the relevant amount before settlement.

---

## 4. Repository Structure

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

## 5. Quick Start for Reviewers

### Prerequisites

- **Go** 1.26+
- **Node** 20+ and **pnpm** 10+
- **Foundry** (`forge`, `cast`)
- **PostgreSQL** 14+ (local or Dockerised)
- A funded **0G Mainnet** wallet for each demo agent (native 0G for gas)

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

| Variable | Purpose |
| --- | --- |
| `ZG_EVM_RPC` | `https://evmrpc.0g.ai` |
| `ZG_STORAGE_INDEXER_RPC` | 0G Storage indexer endpoint |
| `DB_*` | Postgres connection (host / port / user / password / name) |
| `ESCROW_CONTRACT_ADDRESS` | See deployed contracts table above |
| `NETTING_SETTLEMENT_ADDRESS` | See deployed contracts table above |
| `NETTING_SETTLER_PRIVATE_KEY` | Backend's settler EOA (only key the backend holds) |
| `NETTING_WINDOW_SECONDS` | Batch close interval (5 for demo, 60 default) |
| `QUICKNODE_WEBHOOK_SECRET` | HMAC secret for webhook verification |
| `CORS_ALLOWED_ORIGINS` | Comma-separated origins for the dashboard |

### 3. Create local demo configs

```bash
cp demo/escrow.example.yaml  demo/escrow.local.yaml
cp demo/netting.example.yaml demo/netting.local.yaml
```

Fill in `private_key` for each agent. Every agent wallet needs a small amount of native 0G for gas (the demo CLI auto-mints test rUSD if an agent's balance is below the required volume).

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

Each demo prints the on-chain settlement transaction along with its 0G Explorer URL.

---

## 6. Demo Flow

### Escrow demo — `make demo-escrow`

Two agents: a **Data Buyer** and a **Data Processor**.

1. Buyer creates a work order; spec is uploaded to 0G Storage and `specHash` pinned on-chain.
2. Buyer funds the order on `Escrow.sol` (rUSD locked, `transferFrom` from buyer's wallet).
3. Processor signs `deliver:<orderID>:<deliveryHash>` and submits the proof.
4. Escrow verifies the signature and releases payment to the processor.
5. Backend writes a double-entry journal entry per state change, hashes pinned to 0G Storage.

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

The CLI polls Postgres until the batch is settled and prints the settlement tx hash with its 0G Explorer URL.

🎬 Demo video: <!-- TODO: link -->

---

## 7. Roadmap

**V2**

- 0G Compute TEE-attested delivery verification (replace signature-only proofs).
- ERC-7857 iNFT-backed agent identity (replaces lightweight `agent_id_0g` strings).
- Real USDC settlement (replaces rUSD mock).
- On-chain dispute resolution module.

**V3**

- Multi-asset settlement (any ERC-20 / native).
- Cross-chain settlement with bridge integrations.
- Agent-side SDK in Go and TypeScript.

---

## 8. License & Contact

**License:** MIT <!-- TODO: add LICENSE file at root -->

Built by **Harun** ([@harundarat](https://github.com/harundarat)) for the **0G APAC Hackathon 2026 — Track 3**.
