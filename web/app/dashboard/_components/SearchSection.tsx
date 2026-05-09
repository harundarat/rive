"use client";

import { Icon } from "./icons";
import { DEMO_AGENTS } from "./data";

interface Props {
  input: string;
  setInput: (v: string) => void;
  loading: boolean;
  onSubmit: (addr: string) => void;
}

export function SearchSection({ input, setInput, loading, onSubmit }: Props) {
  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    onSubmit(input.trim());
  }

  return (
    <section className="db-search-wrap">
      <div className="db-search-inner">
        <div className="db-section-label">P&amp;L Statement · Read-only</div>
        <h1 className="db-search-h1">
          Inspect any AI agent&apos;s books — anchored on-chain.
        </h1>
        <p className="db-search-sub">
          Enter an agent wallet address to see its live profit and loss, expense
          breakdown, work-order history, and the 0G Storage hashes that anchor
          every journal batch.
        </p>
        <form className="db-search-bar" onSubmit={handleSubmit}>
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="0x26Dea28e89DFdF4Cd5ab9f63010Bb46316Ec3a73"
            spellCheck={false}
          />
          <button type="submit">
            {loading ? (
              "Loading…"
            ) : (
              <>
                Load report <Icon.arrow style={{ marginLeft: 4 }} />
              </>
            )}
          </button>
        </form>
        <div className="db-demo-row">
          <span className="db-mono" style={{ fontSize: 11 }}>
            Try:
          </span>
          {DEMO_AGENTS.map((a) => (
            <button
              key={a.address}
              className="db-demo-pill"
              onClick={() => onSubmit(a.address)}
            >
              <span
                className={`db-dot ${a.status === "profitable" ? "green" : "red"}`}
              />
              {a.label} · {a.address.slice(0, 6)}…{a.address.slice(-4)}
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
