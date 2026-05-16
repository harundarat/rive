# Netting Demo

This demo submits 20 off-chain payment intents and waits for Rive to compress them into one on-chain `NettingSettlement` transaction.

## Prerequisites

Backend must already be running. Use a short netting window for recording:

```sh
NETTING_SETTLEMENT_ADDRESS=0x...
NETTING_SETTLER_PRIVATE_KEY=0x...
NETTING_WINDOW_SECONDS=5
```

The demo script does not start the backend and does not run migrations. Make sure the database has been migrated or reset with the current migrations.

## Configure Wallets

Create a local YAML file from the tracked example:

```sh
cp demo/netting.example.yaml demo/netting.local.yaml
```

Edit `demo/netting.local.yaml`:

- Set `database` to the same Postgres database used by the backend.
- Set `contracts.rusd` and `contracts.netting_settlement`.
- Replace each agent `private_key`.
- Make sure each agent wallet has enough native 0G gas for `mint` and `approve`.

`demo/*.local.yaml` is ignored by git, so real private keys stay local.

## Run

From the repo root:

```sh
make demo-netting
```

Use a different YAML file:

```sh
make demo-netting CONFIG=demo/netting.mainnet.yaml
```

The script will:

1. Check backend health at `/api/health/`.
2. Upsert demo agents into Postgres.
3. Mint rUSD if an agent balance is below its outgoing demo volume.
4. Approve `NettingSettlement` for each paying agent.
5. Submit 20 `POST /api/payments/intent` requests.
6. Poll Postgres until the batch is settled.
7. Print a terminal-ready summary with the settlement tx and explorer URL.

## Demo Shot

The example config is designed to show:

```text
20 logical payments compressed into 1 on-chain settlement tx
Gross volume: 79 rUSD
Net settlement amount: 23 rUSD
```

For a clean recording, run this against an empty or settled netting queue. The script fails early if existing `pending` or `batched` payment intents are found, because they could be picked up in the same backend netting window.
