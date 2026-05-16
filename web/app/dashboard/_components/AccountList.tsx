import type { AccountEntry } from "./types";
import { strCmp, strAdd, fmtAmount } from "./data";

interface Props {
  kind: "revenue" | "expense";
  items: AccountEntry[];
  decimals: number;
}

export function AccountList({ kind, items, decimals }: Props) {
  const total = items.reduce((s, x) => strAdd(s, x.amount), "0");

  if (!items.length) {
    return (
      <div className="db-empty">
        <strong>No {kind} entries yet</strong>
        {kind === "revenue"
          ? "When this agent is paid for fulfilling a work order, revenue lines will appear here."
          : "When this agent funds an escrow that gets released, expense lines will appear here."}
      </div>
    );
  }

  return (
    <div className={`db-acct-list ${kind}`}>
      {items.map((it, i) => {
        const pct =
          strCmp(total, "0") > 0
            ? (parseFloat(it.amount.slice(0, 14) || "0") / parseFloat(total.slice(0, 14) || "1")) * 100
            : 0;
        return (
          <div className="db-acct-row" key={i}>
            <div>
              <div className="db-acct-name">{it.account}</div>
              <div className="db-acct-meta">
                {it.entryCount} {it.entryCount === 1 ? "entry" : "entries"} · {pct.toFixed(1)}%
              </div>
            </div>
            <div className="db-acct-bar">
              <i style={{ width: `${pct}%` }} />
            </div>
            <div className="db-acct-amt">{fmtAmount(it.amount, decimals)}</div>
          </div>
        );
      })}
    </div>
  );
}
