export interface AccountEntry {
  account: string;
  amount: string;
  entryCount: number;
}

export interface CounterpartyRef {
  role: string;
  address: string;
}

export interface Reference {
  type: string;
  id: string;
}

export interface Transaction {
  journalEntryId: string;
  auditBatchId: string | null;
  source: string;
  account: string;
  accountType: string;
  entryType: "debit" | "credit";
  amount: string;
  counterparty: CounterpartyRef;
  reference: Reference;
  description: string;
  occurredAt: string;
}

export interface AuditBatch {
  batchId: string;
  source: string;
  storageRootHash: string | null;
  entryCount: number;
  anchoredAt: string;
  explorerUrl: string | null;
  chainExplorerUrl: string | null;
}

export interface AuditTrailData {
  journalBatchCount: number;
  batches: AuditBatch[];
}

export interface Summary {
  totalRevenue: string;
  totalExpenses: string;
  netIncome: string;
  transactionCount: number;
}

export interface Agent {
  address: string;
  registeredAt: string;
}

export interface Period {
  from: string | null;
  to: string;
}

export interface AgentData {
  agent: Agent;
  period: Period;
  asset: string;
  decimals: number;
  summary: Summary;
  revenue: AccountEntry[];
  expenses: AccountEntry[];
  transactions: Transaction[];
  auditTrail: AuditTrailData;
  generatedAt: string;
  version: string;
}

export interface AgentResponse {
  success: boolean;
  data: AgentData;
}

export interface DemoAgent {
  label: string;
  status: "profitable" | "loss";
  address: string;
}
