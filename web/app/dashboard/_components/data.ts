import type { AgentResponse, DemoAgent, WorkOrder } from "./types";

// ---- Formatters ----

export function fmtAmount(rawStr: string | null | undefined, decimals = 18, opts: { showSign?: boolean; max?: number } = {}): string {
  const { showSign = false, max = 4 } = opts;
  if (rawStr == null) return "—";
  const neg = rawStr.startsWith("-");
  const abs = (neg ? rawStr.slice(1) : rawStr).replace(/^0+(?=\d)/, "") || "0";
  if (!/^\d+$/.test(abs)) return rawStr;
  const padded = abs.padStart(decimals + 1, "0");
  const whole = padded.slice(0, padded.length - decimals);
  const fracRaw = padded.slice(padded.length - decimals);
  const fracStr = fracRaw.slice(0, max).replace(/0+$/, "");
  const wholeFmt = whole.replace(/^0+(?=\d)/, "").replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  let s = fracStr.length ? `${wholeFmt}.${fracStr}` : wholeFmt;
  const isZero = whole.replace(/^0+/, "") === "" && fracStr === "";
  if (neg) s = "-" + s;
  else if (showSign && !isZero) s = "+" + s;
  return s;
}

export function strCmp(a: string, b: string): number {
  a = a.replace(/^0+(?=\d)/, "") || "0";
  b = b.replace(/^0+(?=\d)/, "") || "0";
  if (a.length !== b.length) return a.length < b.length ? -1 : 1;
  return a < b ? -1 : a > b ? 1 : 0;
}

export function strAdd(a: string, b: string): string {
  let i = a.length - 1, j = b.length - 1, carry = 0, out = "";
  while (i >= 0 || j >= 0 || carry) {
    const s = (i >= 0 ? +a[i--] : 0) + (j >= 0 ? +b[j--] : 0) + carry;
    out = (s % 10) + out;
    carry = Math.floor(s / 10);
  }
  return out.replace(/^0+(?=\d)/, "");
}

export function shortAddr(a: string | undefined | null): string {
  if (!a) return "—";
  return a.length > 12 ? `${a.slice(0, 6)}…${a.slice(-4)}` : a;
}

export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleString("en-US", { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleDateString("en-US", { month: "short", day: "2-digit", year: "numeric" });
}

export function relTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

// ---- Mock data ----

function tx(
  entryType: "debit" | "credit",
  account: string,
  accountType: string,
  amount: string,
  description: string,
  occurredAt: string,
  cpAddr: string,
  refType: string,
  refId: string
) {
  return {
    journalEntryId: refId,
    auditBatchId: refId,
    source: refType === "settlement" ? "netting_settle" : "escrow_event",
    account, accountType, entryType, amount,
    counterparty: { role: entryType === "debit" ? "payee" : "payer", address: cpAddr },
    reference: { type: refType, id: refId },
    description, occurredAt,
  };
}

function ab(id: string, source: string, root: string, n: number, anchoredAt: string, chainUrl: string | null) {
  return {
    batchId: id, source,
    storageRootHash: root, entryCount: n, anchoredAt,
    explorerUrl: `https://storagescan.0g.ai/tx/${root}`,
    chainExplorerUrl: chainUrl || null,
  };
}

export const SAMPLE_RESPONSE: AgentResponse = {
  success: true,
  data: {
    agent: { address: "0x26Dea28e89DFdF4Cd5ab9f63010Bb46316Ec3a73", registeredAt: "2026-05-07T23:32:29Z" },
    period: { from: null, to: "2026-05-08T13:39:44Z" },
    asset: "rUSD",
    decimals: 18,
    summary: { totalRevenue: "0", totalExpenses: "10000000000000000000", netIncome: "-10000000000000000000", transactionCount: 1 },
    revenue: [],
    expenses: [{ account: "Service Expense", amount: "10000000000000000000", entryCount: 1 }],
    transactions: [{
      journalEntryId: "019e04ca-1719-7973-bba1-d5b9c426f75c",
      auditBatchId: "019e04ca-1719-7973-bba1-d5b9c426f75c",
      source: "escrow_event",
      account: "Service Expense",
      accountType: "expense",
      entryType: "debit",
      amount: "10000000000000000000",
      counterparty: { role: "payee", address: "0x933a54D5D7a6c0c9e6318395a74cb99Ac1C56934" },
      reference: { type: "work_order", id: "019e04c9-4812-7794-ba5b-173b082c987b" },
      description: "Escrow order released for on-chain order 6",
      occurredAt: "2026-05-07T23:33:33Z",
    }],
    auditTrail: {
      journalBatchCount: 1,
      batches: [{
        batchId: "019e04ca-1719-7973-bba1-d5b9c426f75c",
        source: "escrow_event",
        storageRootHash: "0x009dc3c8d316b60ec3b39d3877ec0e9508ffeddf42b65cce94530017d05b2af2",
        entryCount: 1,
        anchoredAt: "2026-05-07T23:33:33Z",
        explorerUrl: "https://storagescan.0g.ai/tx/0x009dc3c8d316b60ec3b39d3877ec0e9508ffeddf42b65cce94530017d05b2af2",
        chainExplorerUrl: null,
      }],
    },
    generatedAt: "2026-05-08T13:39:44Z",
    version: "1.0",
  },
};

export const PROFITABLE_RESPONSE: AgentResponse = {
  success: true,
  data: {
    agent: { address: "0x933a54D5D7a6c0c9e6318395a74cb99Ac1C56934", registeredAt: "2026-04-12T08:14:02Z" },
    period: { from: "2026-04-12T00:00:00Z", to: "2026-05-08T13:39:44Z" },
    asset: "rUSD",
    decimals: 18,
    summary: {
      totalRevenue: "184500000000000000000",
      totalExpenses: "47200000000000000000",
      netIncome: "137300000000000000000",
      transactionCount: 23,
    },
    revenue: [
      { account: "Service Revenue — Inference", amount: "112000000000000000000", entryCount: 14 },
      { account: "Service Revenue — Data Fetch", amount: "52500000000000000000", entryCount: 6 },
      { account: "Netting Settlement Income", amount: "20000000000000000000", entryCount: 3 },
    ],
    expenses: [
      { account: "Service Expense — Compute", amount: "31200000000000000000", entryCount: 9 },
      { account: "Gas / Network Fees", amount: "10800000000000000000", entryCount: 12 },
      { account: "Service Expense — Storage", amount: "5200000000000000000", entryCount: 2 },
    ],
    transactions: [
      tx("debit",  "Service Expense — Compute",      "expense", "9000000000000000000",  "Compute lease released for batch BX-228",    "2026-05-08T11:02:14Z", "0x26De…3a73", "work_order",  "019e0531-aa11-1112-bb22-aa00d2"),
      tx("credit", "Service Revenue — Inference",     "revenue", "12000000000000000000", "Inference order 142 confirmed",              "2026-05-08T09:48:07Z", "0xA1B2…c7D9", "work_order",  "019e0530-91aa-1234-bb22-aa00d3"),
      tx("debit",  "Gas / Network Fees",              "expense", "1200000000000000000",  "Batch settlement gas — netting cycle 17",    "2026-05-08T08:30:00Z", "0x0G00…pool", "settlement",  "019e0530-1234-aaaa-1111-aa00d4"),
      tx("credit", "Service Revenue — Data Fetch",    "revenue", "8500000000000000000",  "Dataset stream order 87 confirmed",          "2026-05-07T22:12:55Z", "0xC3F1…22aa", "work_order",  "019e052f-aaaa-bbbb-cccc-aa00d5"),
      tx("credit", "Netting Settlement Income",       "revenue", "20000000000000000000", "Netting cycle 17 net-credit settled",        "2026-05-07T20:00:00Z", "0x0G00…pool", "settlement",  "019e052f-1111-2222-3333-aa00d6"),
      tx("debit",  "Service Expense — Compute",       "expense", "6200000000000000000",  "Compute lease released for batch BX-227",    "2026-05-07T15:42:11Z", "0x55aa…77bb", "work_order",  "019e052e-aaaa-bbbb-cccc-aa00d7"),
      tx("credit", "Service Revenue — Inference",     "revenue", "16500000000000000000", "Inference order 141 confirmed",              "2026-05-07T13:17:30Z", "0xA1B2…c7D9", "work_order",  "019e052e-1111-2222-3333-aa00d8"),
    ],
    auditTrail: {
      journalBatchCount: 4,
      batches: [
        ab("019e0531-aa11-1112-bb22-aa00d2", "escrow_event",   "0x7a3e8d92f1a4c0e4b8f9d2c7e6a5b4f3e2d1c0b9a8e7d6c5b4a3f2e1d0c9b8a7", 8, "2026-05-08T11:02:50Z", "https://chainscan.0g.ai/tx/0x7a3e8d92f1a4c0e4b8f9d2c7e6a5b4f3e2d1c0b9a8e7d6c5b4a3f2e1d0c9b8a7"),
        ab("019e0530-91aa-1234-bb22-aa00d3", "escrow_event",   "0xb1c2d3e4f5061728394a5b6c7d8e9f0a1b2c3d4e5f60718293a4b5c6d7e8f900", 6, "2026-05-08T09:48:30Z", null),
        ab("019e0530-1234-aaaa-1111-aa00d4", "netting_settle", "0xee99aa11bb22cc33dd44ee55ff6677889900aabbccddeeff112233445566aabb", 5, "2026-05-08T08:30:20Z", "https://chainscan.0g.ai/tx/0xee99aa11bb22cc33dd44ee55ff6677889900aabbccddeeff112233445566aabb"),
        ab("019e052f-aaaa-bbbb-cccc-aa00d5", "escrow_event",   "0x12abcd34ef5678901234567890abcdef1234567890abcdef1234567890abcdef", 4, "2026-05-07T22:13:18Z", null),
      ],
    },
    generatedAt: "2026-05-08T13:39:44Z",
    version: "1.0",
  },
};

export const DEMO_AGENTS: DemoAgent[] = [
  { label: "Inference agent", status: "profitable", address: PROFITABLE_RESPONSE.data.agent.address, response: PROFITABLE_RESPONSE },
  { label: "Buyer agent",     status: "loss",       address: SAMPLE_RESPONSE.data.agent.address,    response: SAMPLE_RESPONSE },
];

export const MOCK_WORK_ORDERS: Record<string, WorkOrder[]> = {
  [PROFITABLE_RESPONSE.data.agent.address]: [
    { id: "019e0531-aa11-…aa00d2", state: "Released", counterparty: "0x26De…3a73", role: "payee", amount: "9000000000000000000",  asset: "rUSD", updated: "2026-05-08T11:02:14Z" },
    { id: "019e0530-91aa-…aa00d3", state: "Released", counterparty: "0xA1B2…c7D9", role: "payee", amount: "12000000000000000000", asset: "rUSD", updated: "2026-05-08T09:48:07Z" },
    { id: "019e0530-1234-…aa00d4", state: "Funded",   counterparty: "0x0G00…pool", role: "payer", amount: "1200000000000000000",  asset: "rUSD", updated: "2026-05-08T08:30:00Z" },
    { id: "019e052f-aaaa-…aa00d5", state: "Released", counterparty: "0xC3F1…22aa", role: "payee", amount: "8500000000000000000",  asset: "rUSD", updated: "2026-05-07T22:12:55Z" },
    { id: "019e052e-aaaa-…aa00d7", state: "Refunded", counterparty: "0x55aa…77bb", role: "payer", amount: "6200000000000000000",  asset: "rUSD", updated: "2026-05-07T15:42:11Z" },
    { id: "019e052e-1111-…aa00d8", state: "Draft",    counterparty: "0xA1B2…c7D9", role: "payee", amount: "16500000000000000000", asset: "rUSD", updated: "2026-05-07T13:17:30Z" },
  ],
  [SAMPLE_RESPONSE.data.agent.address]: [
    { id: "019e04c9-4812-…2c987b", state: "Released", counterparty: "0x933a…6934", role: "payer", amount: "10000000000000000000", asset: "rUSD", updated: "2026-05-07T23:33:33Z" },
  ],
};
