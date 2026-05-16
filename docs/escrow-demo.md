# Escrow Demo

This demo runs one work order through the full escrow lifecycle:

```text
draft -> funded -> delivery submitted -> completed
```

The script drives the backend and contracts, then relays escrow transaction receipts to the existing QuickNode webhook endpoint locally. That keeps the demo deterministic while still exercising the same backend event path used in production.

## Prerequisites

Backend must already be running with escrow webhook config:

```sh
QUICKNODE_WEBHOOK_SECRET=...
ESCROW_CONTRACT_ADDRESS=0x...
```

The demo script does not start the backend and does not run migrations. Make sure the database has been migrated or reset with the current migrations.

## Configure Wallets

Create a local YAML file from the tracked example:

```sh
cp demo/escrow.example.yaml demo/escrow.local.yaml
```

Edit `demo/escrow.local.yaml`:

- Set `database` to the same Postgres database used by the backend.
- Set `quicknode_webhook_secret` to the same value as backend `QUICKNODE_WEBHOOK_SECRET`.
- Set `contracts.rusd` and `contracts.escrow`.
- Replace payer and payee `private_key`.
- Make sure the payer and payee satisfy `minimum_native_balance_wei`; the payer sends the demo transactions and the payee signs delivery.

`demo/*.local.yaml` is ignored by git, so real private keys stay local.

## Run

From the repo root:

```sh
make demo-escrow
```

Use a different YAML file:

```sh
make demo-escrow CONFIG=demo/escrow.mainnet.yaml
```

The script will:

1. Check backend health at `/api/health/`.
2. Upsert payer and payee agents into Postgres.
3. Mint rUSD to the payer if needed.
4. Approve `Escrow` for the configured amount.
5. Create a work order through `POST /api/work-orders`.
6. Call `Escrow.createOrder`.
7. Relay the `OrderCreated` receipt to `/api/webhooks/quicknode/escrow-events`.
8. Upload delivery JSON through `/api/storage/upload`.
9. Submit a payee signature to `/api/work-orders/{onchainOrderID}/delivery`.
10. Call `Escrow.releaseOrder`.
11. Relay the `OrderReleased` receipt to the backend webhook.
12. Print a terminal-ready summary with create/release transaction links.

## Demo Shot

The expected final headline is:

```text
Work order funded, delivered, and released through escrow.
```

For a clean recording, use a backend connected to the same database and escrow contract configured in the YAML. The local webhook relay signs the receipt payload with `quicknode_webhook_secret`, so a mismatch with backend `.env` will fail before the work order status changes.
