"use client";

import { useState, useMemo, useCallback } from "react";
import type { Transaction } from "./types";
import { fmtAmount, fmtTime, relTime, shortAddr } from "./data";
import { Icon } from "./icons";

interface Props {
  txs: Transaction[];
  decimals: number;
}

export function TransactionsTable({ txs, decimals }: Props) {
  const [filter, setFilter] = useState<"all" | "revenue" | "expense">("all");
  const PAGE_SIZE = 10;
  const [page, setPage] = useState(1);

  const filtered = useMemo(() => {
    if (filter === "all") return txs;
    if (filter === "revenue")
      return txs.filter((t) => t.accountType === "revenue");
    return txs.filter((t) => t.accountType === "expense");
  }, [txs, filter]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginated = useMemo(
    () => filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE),
    [filtered, safePage],
  );

  const revCount = txs.filter((t) => t.accountType === "revenue").length;
  const expCount = txs.filter((t) => t.accountType === "expense").length;

  function changeFilter(f: "all" | "revenue" | "expense") {
    setFilter(f);
    setPage(1);
  }

  const exportCSV = useCallback(() => {
    const COLUMNS = [
      "Time",
      "Entry",
      "Account",
      "Account Type",
      "Description",
      "Reference Type",
      "Reference ID",
      "Counterparty Address",
      "Counterparty Role",
      "Amount (rUSD)",
      "Source",
    ];

    function csvCell(value: string): string {
      if (value.includes('"') || value.includes(",") || value.includes("\n")) {
        return '"' + value.replace(/"/g, '""') + '"';
      }
      return value;
    }

    const rows: string[] = [COLUMNS.join(",")];
    for (const t of filtered) {
      rows.push(
        [
          fmtTime(t.occurredAt),
          t.entryType,
          t.account,
          t.accountType,
          t.description,
          t.reference.type,
          t.reference.id,
          t.counterparty.address,
          t.counterparty.role,
          fmtAmount(t.amount, decimals),
          t.source,
        ]
          .map(csvCell)
          .join(","),
      );
    }

    const csv = rows.join("\r\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const today = new Date().toISOString().slice(0, 10);
    const filename = `rive-txs-${filter}-${today}.csv`;

    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 100);
  }, [filtered, filter, decimals]);

  return (
    <div className="db-card">
      <div className="db-card-head">
        <div>
          <div className="db-section-label">Journal entries</div>
          <div className="db-card-title">Transactions</div>
        </div>
        <div className="db-chip-row">
          <button
            className={`db-chip${filter === "all" ? " active" : ""}`}
            onClick={() => changeFilter("all")}
          >
            All <span className="db-chip-count">{txs.length}</span>
          </button>
          <button
            className={`db-chip${filter === "revenue" ? " active" : ""}`}
            onClick={() => changeFilter("revenue")}
          >
            Revenue <span className="db-chip-count">{revCount}</span>
          </button>
          <button
            className={`db-chip${filter === "expense" ? " active" : ""}`}
            onClick={() => changeFilter("expense")}
          >
            Expense <span className="db-chip-count">{expCount}</span>
          </button>
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="db-empty">
          <strong>No matching entries</strong>
          Try changing the filter or expanding the period.
        </div>
      ) : (
        <div style={{ overflow: "auto" }}>
          <table className="db-tx">
            <thead>
              <tr>
                <th style={{ width: "14%" }}>Time</th>
                <th style={{ width: "10%" }}>Entry</th>
                <th style={{ width: "22%" }}>Account</th>
                <th>Description</th>
                <th style={{ width: "16%" }}>Counterparty</th>
                <th style={{ width: "12%", textAlign: "right" }}>Amount</th>
                <th style={{ width: "10%" }}>Source</th>
              </tr>
            </thead>
            <tbody>
              {paginated.map((t, i) => {
                const sign = t.entryType === "debit" ? "-" : "+";
                const tone = t.entryType === "debit" ? "neg" : "pos";
                return (
                  <tr key={i}>
                    <td>
                      <div style={{ fontSize: 13 }}>
                        {fmtTime(t.occurredAt)}
                      </div>
                      <div style={{ fontSize: 11, color: "var(--db-ink-60)" }}>
                        {relTime(t.occurredAt)}
                      </div>
                    </td>
                    <td>
                      <span className={`db-entry-pill ${t.entryType}`}>
                        {t.entryType}
                      </span>
                    </td>
                    <td>
                      <div style={{ fontSize: 14 }}>{t.account}</div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--db-ink-60)",
                          textTransform: "capitalize",
                        }}
                      >
                        {t.accountType}
                      </div>
                    </td>
                    <td>
                      <div style={{ fontSize: 13 }}>{t.description}</div>
                      <div className="db-ref-id">
                        {t.reference.type.replace("_", " ")} ·{" "}
                        {t.reference.id.slice(0, 8)}…{t.reference.id.slice(-4)}
                      </div>
                    </td>
                    <td>
                      <a
                        className="db-addr"
                        href="#"
                        onClick={(e) => e.preventDefault()}
                        title={t.counterparty.address}
                      >
                        {shortAddr(t.counterparty.address)}
                      </a>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--db-ink-60)",
                          marginTop: 2,
                          textTransform: "capitalize",
                        }}
                      >
                        as {t.counterparty.role}
                      </div>
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <div className={`db-amt ${tone}`}>
                        {sign}
                        {fmtAmount(t.amount, decimals)}
                      </div>
                      <div style={{ fontSize: 11, color: "var(--db-ink-60)" }}>
                        rUSD
                      </div>
                    </td>
                    <td>
                      <span
                        className="db-entry-pill"
                        style={{ background: "var(--db-ink-04)" }}
                      >
                        {t.source.replace("_", " ")}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {filtered.length > PAGE_SIZE ? (
        <div className="db-pagination">
          <div className="db-pagination-info">
            <span className="db-pagination-text">
              Showing {(safePage - 1) * PAGE_SIZE + 1}–
              {Math.min(safePage * PAGE_SIZE, filtered.length)} of{" "}
              {filtered.length}
            </span>
            <div className="db-pagination-nav">
              <button
                className="db-btn db-btn-ghost"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={safePage === 1}
                aria-label="Previous page"
              >
                ← Prev
              </button>
              <span className="db-pagination-count">
                {safePage} / {totalPages}
              </span>
              <button
                className="db-btn db-btn-ghost"
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={safePage === totalPages}
                aria-label="Next page"
              >
                Next →
              </button>
            </div>
          </div>
          <button
            className="db-btn db-btn-outline"
            onClick={exportCSV}
            disabled={filtered.length === 0}
            title={`Export CSV`}
          >
            <Icon.download /> Export {filtered.length} rows as CSV
          </button>
        </div>
      ) : (
        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            marginTop: 16,
            paddingTop: 16,
            borderTop: "1px solid var(--db-ink-08)",
          }}
        >
          <button
            className="db-btn db-btn-outline"
            onClick={exportCSV}
            disabled={filtered.length === 0}
            title={filtered.length === 0 ? "No rows to export" : `Export CSV`}
          >
            <Icon.download /> Export {filtered.length} rows as CSV
          </button>
        </div>
      )}
    </div>
  );
}
