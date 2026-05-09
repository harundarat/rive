"use client";

import "./dashboard.css";
import { useCallback, useEffect, useRef, useState } from "react";
import { fmtAmount } from "./_components/data";
import {
  DEFAULT_WALLET_ADDRESS,
  PnLApiError,
  fetchPnLReport,
} from "./_components/api";
import type { AgentResponse } from "./_components/types";
import { Nav } from "./_components/Nav";
import { SearchSection } from "./_components/SearchSection";
import { ReportHead } from "./_components/ReportHead";
import { StatCards } from "./_components/StatCards";
import { AccountList } from "./_components/AccountList";
import { TransactionsTable } from "./_components/TransactionsTable";
import { AuditTrail } from "./_components/AuditTrail";
import { Toast } from "./_components/Toast";

export default function DashboardPage() {
  const [input, setInput] = useState(DEFAULT_WALLET_ADDRESS);
  const [response, setResponse] = useState<AgentResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const loadReport = useCallback(async (addr: string) => {
    const walletAddress = addr.trim();
    if (!walletAddress) {
      setError("Enter a wallet address to load a P&L report.");
      return;
    }

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setInput(walletAddress);
    setLoading(true);
    setError(null);

    try {
      const nextResponse = await fetchPnLReport(
        walletAddress,
        controller.signal,
      );
      setResponse(nextResponse);
      setInput(nextResponse.data.agent.address);
    } catch (err) {
      if (controller.signal.aborted) return;
      const message =
        err instanceof PnLApiError
          ? err.message
          : "Unable to connect to the P&L backend.";
      setError(message);
      window.dispatchEvent(new CustomEvent("rive-toast", { detail: message }));
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    const timeoutId = window.setTimeout(() => {
      void loadReport(DEFAULT_WALLET_ADDRESS);
    }, 0);

    return () => {
      window.clearTimeout(timeoutId);
      abortRef.current?.abort();
    };
  }, [loadReport]);

  function submit(addr: string) {
    void loadReport(addr);
  }

  const data = response?.data;

  return (
    <div className="db-page">
      <Toast />
      <Nav />
      <SearchSection input={input} setInput={setInput} loading={loading} onSubmit={submit} />

      <main className="db-page-content full">
        {error && (
          <div className="db-alert" role="status" aria-live="polite">
            <strong>Could not load report</strong>
            <span>{error}</span>
          </div>
        )}

        {!data ? (
          <div className="db-card db-loading-card">
            <div className="db-section-label">Agent P&amp;L</div>
            <div className="db-card-title">
              {loading ? "Loading report" : "No report loaded"}
            </div>
            <p>
              {loading
                ? "Fetching the latest ledger summary from the backend."
                : "Enter an agent wallet address to load its live books."}
            </p>
          </div>
        ) : (
          <>
            <ReportHead data={data} />

            <div style={{ marginTop: 24 }}>
              <div className="db-section-label">
                Summary · {data.period.from ? "since registration" : "all-time"}
              </div>
              <StatCards
                summary={data.summary}
                revenue={data.revenue}
                expenses={data.expenses}
                auditTrail={data.auditTrail}
                decimals={data.decimals}
              />
            </div>

            <div style={{ marginTop: 24 }}>
              <div className="db-kpi-grid">
                <div className="db-card">
                  <div className="db-card-head">
                    <div>
                      <div className="db-section-label">Revenue accounts</div>
                      <div className="db-card-title">Where this agent earns</div>
                    </div>
                    <div className="db-amt pos">{fmtAmount(data.summary.totalRevenue, data.decimals)} rUSD</div>
                  </div>
                  <AccountList kind="revenue" items={data.revenue} decimals={data.decimals} />
                </div>
                <div className="db-card">
                  <div className="db-card-head">
                    <div>
                      <div className="db-section-label">Expense accounts</div>
                      <div className="db-card-title">Where this agent spends</div>
                    </div>
                    <div className="db-amt neg">{fmtAmount(data.summary.totalExpenses, data.decimals)} rUSD</div>
                  </div>
                  <AccountList kind="expense" items={data.expenses} decimals={data.decimals} />
                </div>
              </div>
            </div>

            <div style={{ marginTop: 24 }}>
              <TransactionsTable txs={data.transactions} decimals={data.decimals} />
            </div>

            <AuditTrail trail={data.auditTrail} />

            <div className="db-footer">
              <span>Rive Protocol · v{data.version}</span>
              <span>Books anchored on 0G Storage · read-only</span>
            </div>
          </>
        )}
      </main>
    </div>
  );
}
