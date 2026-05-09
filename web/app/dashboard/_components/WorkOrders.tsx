"use client";

import { useState, useMemo } from "react";
import type { WorkOrder } from "./types";
import { fmtAmount, fmtTime, relTime } from "./data";

interface Props {
  orders: WorkOrder[];
  decimals: number;
}

type StateFilter = "all" | "Released" | "Funded" | "Draft" | "Refunded";
const STATES: StateFilter[] = ["all", "Released", "Funded", "Draft", "Refunded"];

export function WorkOrders({ orders, decimals }: Props) {
  const [state, setState] = useState<StateFilter>("all");
  const PAGE_SIZE = 10;
  const [page, setPage] = useState(1);

  const counts = STATES.reduce<Record<string, number>>((m, s) => {
    m[s] = s === "all" ? orders.length : orders.filter((o) => o.state === s).length;
    return m;
  }, {});

  const filtered = useMemo(
    () => (state === "all" ? orders : orders.filter((o) => o.state === state)),
    [orders, state]
  );
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginated = useMemo(
    () => filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE),
    [filtered, safePage]
  );

  function changeState(s: StateFilter) {
    setState(s);
    setPage(1);
  }

  return (
    <div className="db-card" style={{ marginTop: 24 }}>
      <div className="db-card-head">
        <div>
          <div className="db-section-label">Escrow</div>
          <div className="db-card-title">Work orders</div>
        </div>
        <div className="db-chip-row">
          {STATES.map((s) => (
            <button key={s} className={`db-chip${state === s ? " active" : ""}`} onClick={() => changeState(s)}>
              {s === "all" ? "All" : s} <span className="db-chip-count">{counts[s]}</span>
            </button>
          ))}
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="db-empty">
          <strong>No work orders in this state</strong>
          Try a different filter.
        </div>
      ) : (
        <>
          <table className="db-tx">
            <thead>
              <tr>
                <th style={{ width: "24%" }}>Order ID</th>
                <th style={{ width: "12%" }}>State</th>
                <th>Counterparty</th>
                <th style={{ width: "10%" }}>Role</th>
                <th style={{ width: "14%", textAlign: "right" }}>Amount</th>
                <th style={{ width: "14%" }}>Updated</th>
              </tr>
            </thead>
            <tbody>
              {paginated.map((o, i) => (
                <tr key={i}>
                  <td><span className="db-addr">{o.id}</span></td>
                  <td><span className={`db-state ${o.state.toLowerCase()}`}>{o.state}</span></td>
                  <td>
                    <a className="db-addr" href="#" onClick={(e) => e.preventDefault()}>{o.counterparty}</a>
                  </td>
                  <td style={{ fontSize: 13, textTransform: "capitalize" }}>{o.role}</td>
                  <td style={{ textAlign: "right" }} className="db-amt">
                    {fmtAmount(o.amount, decimals)}{" "}
                    <span style={{ color: "var(--db-ink-60)", fontSize: 11 }}>{o.asset}</span>
                  </td>
                  <td>
                    <div style={{ fontSize: 13 }}>{fmtTime(o.updated)}</div>
                    <div style={{ fontSize: 11, color: "var(--db-ink-60)" }}>{relTime(o.updated)}</div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {filtered.length > PAGE_SIZE && (
            <div className="db-pagination">
              <div className="db-pagination-info">
                <span className="db-pagination-text">
                  Showing {(safePage - 1) * PAGE_SIZE + 1}–{Math.min(safePage * PAGE_SIZE, filtered.length)} of {filtered.length}
                </span>
                <div className="db-pagination-nav">
                  <button className="db-btn db-btn-ghost" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={safePage === 1} aria-label="Previous page">← Prev</button>
                  <span className="db-pagination-count">{safePage} / {totalPages}</span>
                  <button className="db-btn db-btn-ghost" onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={safePage === totalPages} aria-label="Next page">Next →</button>
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
