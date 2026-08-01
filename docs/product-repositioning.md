# Rive Product Repositioning

> **Rive is the financial operations layer for AI agents.**

## 1. Product Direction

Rive is repositioned from a standalone payment and settlement protocol into an **Agent FinOps platform**. Rive helps teams understand, control, and reconcile the money earned and spent by their AI agents.

Rive does not need to replace existing payment rails. It integrates with protocols such as x402 and on-chain stablecoin payments, then turns their payment receipts and transaction data into reliable financial records.

In its first production version, Rive is primarily **read-only and non-custodial**: it observes payment activity, builds a double-entry ledger, and provides financial visibility without holding user funds or private keys.

## 2. The Product After Repositioning

A user connects a wallet or integrates Rive through an API/SDK. Rive then:

1. Ingests signed payment receipts and on-chain transactions.
2. Identifies the agent, counterparty, asset, service, and project behind each payment.
3. Records every event in a double-entry ledger.
4. Shows revenue, expenses, balances, and profit or loss per agent.
5. Reconciles Rive's ledger with on-chain settlement data.
6. Alerts the user about unusual spending, duplicates, or reconciliation mismatches.

The main dashboard answers practical questions such as:

- How much did each agent spend and earn?
- Which APIs, tools, or counterparties cost the most?
- Is an agent still within its spending budget?
- Which payment records do not match on-chain settlement?
- Is an agent, workflow, or project profitable?

## 3. Target Users

Rive initially serves:

- Teams operating multiple autonomous agents.
- Developers building agents that buy paid APIs through x402.
- Providers monetizing APIs or agent services.
- Agent platforms and marketplaces that need financial reporting without building their own ledger.

The first ideal user is a small technical team that already has programmatic stablecoin payments and needs better visibility, attribution, and reconciliation.

## 4. Core MVP

The first usable version should contain:

- Wallet or API-key onboarding with verified ownership.
- x402 payment receipt ingestion.
- EVM stablecoin transaction ingestion, starting with one network and USDC.
- Double-entry ledger and deterministic idempotent processing.
- P&L by agent, wallet, project, and time period.
- Revenue and expense attribution by service or counterparty.
- Reconciliation status between payment receipts and on-chain transactions.
- Configurable spending budgets and basic alerts.
- CSV/JSON export for external accounting or analysis.
- A hosted dashboard and a small TypeScript SDK or HTTP integration guide.

## 5. What Rive Will Not Do Initially

To keep the product focused and safer to adopt, the MVP will not:

- Hold user funds or private keys.
- Launch a new stablecoin or require rUSD.
- Replace x402, AP2, wallets, or existing payment facilitators.
- Support every chain and token.
- Provide automatic multilateral settlement with real funds.
- Claim permanent decentralized storage unless the data is verifiably anchored there.
- Provide tax, legal, or regulated accounting advice.

The existing escrow and netting engines remain experimental modules and portfolio assets. They are not part of the initial production promise.

## 6. Positioning

### Old positioning

> The settlement layer for AI agents.

### New positioning

> The financial operations layer for AI agents.

Suggested short description:

> Rive gives AI-agent teams a real-time ledger, P&L, spending controls, and on-chain reconciliation for x402 and stablecoin payments.

Rive's differentiation is not merely moving money. Its value is turning fragmented machine payments into financial records that humans and software can understand, audit, and act on.

## 7. Business Model

Rive can begin as a hosted SaaS product with:

- A free developer tier for a small number of agents and transactions.
- Paid tiers based on agents, monthly payment events, or retained history.
- Higher tiers for team access, alerts, exports, custom retention, and marketplace reporting.

The initial goal is to acquire design partners and validate recurring usage before optimizing pricing.

## 8. Product Evolution

### Phase 1 — Visibility

Deliver payment ingestion, double-entry accounting, P&L, and reconciliation for one x402/EVM flow.

### Phase 2 — Control

Add budgets, spending policies, anomaly alerts, team access, and accounting integrations.

### Phase 3 — Optimization

Use real customer transaction data to identify opportunities for cost and liquidity optimization. Multilateral netting may return here as an optional feature for closed agent marketplaces, using signed payment authorizations and a hardened trust model.

## 9. Success Criteria

The repositioning is working when:

- A new team can connect its first payment source without manual database access.
- Rive can explain every displayed balance from source receipt to on-chain settlement.
- Users return regularly to monitor spending or investigate transactions.
- At least three design partners use Rive with real agent payment activity.
- Product decisions are driven by observed payment patterns rather than assumed demand for a new settlement protocol.

---

This repositioning preserves Rive's strongest engineering assets—its Go backend, canonical event processing, double-entry ledger, reconciliation logic, and dashboard—while giving the product a narrower and more realistic path to real users.
