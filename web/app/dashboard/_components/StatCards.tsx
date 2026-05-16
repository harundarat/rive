import type { Summary, AccountEntry, AuditTrailData } from "./types";
import { fmtAmount } from "./data";

interface Props {
  summary: Summary;
  revenue: AccountEntry[];
  expenses: AccountEntry[];
  auditTrail: AuditTrailData;
  decimals: number;
}

function Stat({ label, value, sub, tone, unit = "rUSD" }: {
  label: string;
  value: string | number;
  sub?: string;
  tone?: "pos" | "neg" | null;
  unit?: string | null;
}) {
  return (
    <div className={`db-stat${tone ? ` ${tone}` : ""}`}>
      <div className="db-stat-label">{label}</div>
      <div className="db-stat-value">
        <span>{value}</span>
        {unit && <span className="db-stat-unit">{unit}</span>}
      </div>
      {sub && <div className="db-stat-sub">{sub}</div>}
    </div>
  );
}

export function StatCards({ summary, revenue, expenses, auditTrail, decimals }: Props) {
  const tone = summary.netIncome.startsWith("-") ? "neg" : summary.netIncome === "0" ? null : "pos";

  return (
    <div className="db-stats">
      <Stat
        label="Total revenue"
        value={fmtAmount(summary.totalRevenue, decimals)}
        sub={`${revenue.length} ${revenue.length === 1 ? "account" : "accounts"} · credits`}
        tone="pos"
      />
      <Stat
        label="Total expenses"
        value={fmtAmount(summary.totalExpenses, decimals)}
        sub={`${expenses.length} ${expenses.length === 1 ? "account" : "accounts"} · debits`}
        tone="neg"
      />
      <Stat
        label="Net income"
        value={fmtAmount(summary.netIncome, decimals, { showSign: true })}
        sub={tone === "pos" ? "Profit in period" : tone === "neg" ? "Loss in period" : "Break even"}
        tone={tone}
      />
      <Stat
        label="Transactions"
        value={summary.transactionCount.toLocaleString()}
        unit={null}
        sub={`${auditTrail.journalBatchCount} journal ${auditTrail.journalBatchCount === 1 ? "batch" : "batches"} anchored`}
      />
    </div>
  );
}
