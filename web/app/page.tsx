import Reveal from "./_components/Reveal";
import Counter from "./_components/Counter";
import NettingDemo from "./_components/NettingDemo";
import FlowSteps from "./_components/FlowSteps";

function Logo() {
  return (
    <a href="#" className="logo">
      <span className="logo-mark"></span>
      <span>Rive</span>
    </a>
  );
}

function Nav() {
  return (
    <nav className="nav">
      <div className="wrap nav-inner">
        <Logo />
        <div className="nav-links">
          <a href="#protocol">Protocol</a>
          <a href="#flow">How it works</a>
          <a href="#contracts">Contracts</a>
          <a href="/dashboard">Dashboard</a>
          <a href="#docs">Docs</a>
        </div>
        <div className="nav-cta">
          <a className="btn btn-ghost" href="#github">
            GitHub ↗
          </a>
          <a className="btn btn-primary" href="/dashboard">
            Open dashboard
          </a>
        </div>
      </div>
    </nav>
  );
}

function HeroLedger() {
  return (
    <div className="ledger" role="img" aria-label="Sample journal entry">
      <span className="ledger-tag">Journal · Batch #00184</span>
      <div className="ledger-head">
        <span className="t">
          Work order <span className="id">WO-9F4A.21</span>
        </span>
        <span className="t">2026-04-29 · 14:02 UTC</span>
      </div>
      <div style={{ paddingTop: 6 }}>
        <div className="ledger-row">
          <div className="acct">
            Cash — Escrow Vault<span className="sub">agent · scribe-04</span>
          </div>
          <div className="num dr">— 2,400.00</div>
          <div className="num dim">rUSD</div>
        </div>
        <div className="ledger-row">
          <div className="acct">
            Service Revenue — Inference
            <span className="sub">agent · scribe-04</span>
          </div>
          <div className="num cr">+ 2,400.00</div>
          <div className="num dim">rUSD</div>
        </div>
        <div className="ledger-row">
          <div className="acct">
            Cost of Service — GPU Compute
            <span className="sub">counterparty · forge-12</span>
          </div>
          <div className="num dr">— 1,820.00</div>
          <div className="num dim">rUSD</div>
        </div>
        <div className="ledger-row">
          <div className="acct">
            Net Settlement Payable
            <span className="sub">netting batch · NB-118</span>
          </div>
          <div className="num cr">+ 1,820.00</div>
          <div className="num dim">rUSD</div>
        </div>
      </div>
      <div className="ledger-foot">
        <span>
          Hash · <span className="hash mono">0x9c41…7d3a</span>
        </span>
        <span>Anchored · 0G Storage ✓</span>
      </div>
    </div>
  );
}

function Hero() {
  return (
    <header className="hero">
      <div className="wrap">
        <div className="hero-eyebrow">
          <span className="pulse"></span>
          <span className="label label-ink">LIVE ON 0G MAINNET · V1.0</span>
        </div>
        <div className="hero-grid">
          <div>
            <h1>
              <span className="line">The settlement layer</span>
              <br />
              <span className="alt">for AI agents.</span>
            </h1>
            <p className="hero-sub">
              Trustless escrow, verifiable double-entry bookkeeping, and
              gas-optimized batch settlements for autonomous agents.
            </p>
            <div className="hero-ctas">
              <a className="btn btn-primary" href="/dashboard">
                Open dashboard <span className="btn-arrow">→</span>
              </a>
              <a className="btn btn-ghost" href="#docs">
                Read the spec
              </a>
            </div>
            <div className="hero-meta">
              <div>
                <span className="k">Settlement asset</span>
                <span className="v">
                  RiveUSD{" "}
                  <span className="mono" style={{ opacity: 0.55 }}>
                    (rUSD)
                  </span>
                </span>
              </div>
              <div>
                <span className="k">Built on</span>
                <span className="v">0G · modular AI stack</span>
              </div>
              <div>
                <span className="k">Audit trail</span>
                <span className="v">Hash-anchored to 0G Storage</span>
              </div>
            </div>
          </div>
          <div style={{ paddingTop: 12 }}>
            <HeroLedger />
          </div>
        </div>

        <div className="strip">
          <span className="label">Protocol fabric</span>
          <div className="strip-logos">
            <span>0G Chain</span>
            <span className="dot"></span>
            <span>0G Storage</span>
            <span className="dot"></span>
            <span>ERC-20 · rUSD</span>
            <span className="dot"></span>
            <span>Trustless escrow</span>
            <span className="dot"></span>
            <span>Open spec</span>
          </div>
        </div>
      </div>
    </header>
  );
}

function Features() {
  return (
    <section id="protocol">
      <div className="wrap">
        <div className="section-head">
          <div>
            <span className="label label-ink">§ 01 Protocol</span>
            <h2>
              Three primitives.
              <br />
              <span style={{ opacity: 0.5 }}>One settlement layer.</span>
            </h2>
          </div>
          <p className="lead">
            Agents pay each other constantly — for compute, data, and task
            delegation. Rive handles the custody, accounting, and gas costs so
            agents don&apos;t have to.
          </p>
        </div>

        <Reveal className="features stagger">
          {/* ESCROW */}
          <div className="feature">
            <span className="feature-num">01 · Custody</span>
            <h3>Trustless escrow</h3>
            <p>
              Funds lock on-chain when a work order is created. Released only
              when the payer confirms delivery. No intermediary — the contract
              holds the money.
            </p>
            <Reveal className="feature-art" threshold={0.25}>
              <div className="vault">
                <div className="vault-row">
                  <b>Vault — WO-9F4A.21</b>
                  <span>2,400.00 rUSD</span>
                </div>
                <div className="vault-bar">
                  <div className="vault-fill animate"></div>
                </div>
                <div className="vault-row" style={{ fontSize: 10 }}>
                  <span>Locked at block 18,402,114</span>
                  <span>Time-lock 24h</span>
                </div>
                <div className="vault-states">
                  <span className="pill">Draft</span>
                  <span className="pill on">Funded</span>
                  <span className="pill">Released</span>
                  <span className="pill">Refunded</span>
                </div>
              </div>
              <div className="vault">
                <div className="vault-row">
                  <b>Vault — WO-A2B8.07</b>
                  <span>850.00 rUSD</span>
                </div>
                <div className="vault-bar">
                  <div
                    className="vault-fill animate"
                    style={{ width: "38%", opacity: 0.7 }}
                  ></div>
                </div>
                <div className="vault-row" style={{ fontSize: 10 }}>
                  <span>Locked at block 18,402,291</span>
                  <span>Time-lock 12h</span>
                </div>
                <div className="vault-states">
                  <span className="pill on">Draft</span>
                  <span className="pill">Funded</span>
                  <span className="pill">Released</span>
                  <span className="pill">Refunded</span>
                </div>
              </div>
            </Reveal>
          </div>

          {/* BOOKKEEPING */}
          <div className="feature">
            <span className="feature-num">02 · Accounting</span>
            <h3>Double-entry, by default</h3>
            <p>
              Every escrow event emits a balanced journal entry. Each agent gets
              a live P&L and Balance Sheet, with every batch hash-anchored
              on-chain.
            </p>
            <div className="feature-art">
              <div className="journal">
                <div className="journal-head">
                  <span>Journal · NB-118</span>
                  <span>DR / CR</span>
                </div>
                <div className="journal-row">
                  <span className="acct">Cash · Escrow</span>
                  <span className="num dr-l">2,400.00</span>
                  <span></span>
                </div>
                <div className="journal-row">
                  <span className="acct ind">↳ Revenue · Inference</span>
                  <span></span>
                  <span className="num cr-l">2,400.00</span>
                </div>
                <div className="journal-row">
                  <span className="acct">COGS · GPU</span>
                  <span className="num dr-l">1,820.00</span>
                  <span></span>
                </div>
                <div className="journal-row">
                  <span className="acct ind">↳ Net Payable</span>
                  <span></span>
                  <span className="num cr-l">1,820.00</span>
                </div>
                <div className="journal-row">
                  <span className="acct">Protocol Fee</span>
                  <span className="num dr-l">24.00</span>
                  <span></span>
                </div>
                <div className="journal-row">
                  <span className="acct ind">↳ Fee Revenue</span>
                  <span></span>
                  <span className="num cr-l">24.00</span>
                </div>
                <div className="journal-row">
                  <span className="acct">
                    <b>Σ Balanced</b>
                  </span>
                  <span className="num dr-l">4,244.00</span>
                  <span className="num cr-l">4,244.00</span>
                </div>
              </div>
            </div>
          </div>

          {/* NETTING */}
          <div className="feature">
            <span className="feature-num">03 · Netting</span>
            <h3>Micro-payment netting</h3>
            <p>
              Instead of settling every micro-payment individually, Rive batches
              them. Twenty transactions become three settlements — a fraction of
              the gas cost.
            </p>
            <div className="feature-art">
              <NettingDemo />
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function Flow() {
  return (
    <section id="flow" className="flow-wrap">
      <div className="wrap">
        <div className="section-head">
          <div>
            <span className="label" style={{ color: "rgba(246,244,238,.55)" }}>
              § 02 Lifecycle
            </span>
            <h2>
              From handshake
              <br />
              <span style={{ opacity: 0.55 }}>to hash-anchored close.</span>
            </h2>
          </div>
          <p className="lead">
            Every transaction moves through four states. Each state change
            produces a journal entry; every batch of entries is anchored to 0G
            Storage.
          </p>
        </div>

        <FlowSteps />

        <div id="contracts" style={{ marginTop: 64 }}>
          <div className="section-head" style={{ marginBottom: 24 }}>
            <div>
              <span
                className="label"
                style={{ color: "rgba(246,244,238,.55)" }}
              >
                § 03 On-chain
              </span>
              <h2 style={{ fontSize: "40px" }}>Deployed contracts</h2>
            </div>
            <p className="lead"></p>
          </div>
          <Reveal className="contracts stagger">
            <div className="contract">
              <div className="ct-name">
                <b>RiveEscrow</b>
                <span className="chip">
                  <span className="d g"></span>Verified
                </span>
              </div>
              <span className="ct-tag">
                Custody · Work-order vaults · Time-locks
              </span>
              <div className="ct-addr">
                <span>0xA17b · 4f02 · 9C3d · 7e1B · F29a · 4e58</span>
                <a href="#">0G Explorer ↗</a>
              </div>
            </div>
            <div className="contract">
              <div className="ct-name">
                <b>NettingSettlement</b>
                <span className="chip">
                  <span className="d g"></span>Verified
                </span>
              </div>
              <span className="ct-tag">
                Batch settlement · Multilateral netting
              </span>
              <div className="ct-addr">
                <span>0x2dA9 · 78C3 · 4F1e · 0aB5 · cc44 · 9012</span>
                <a href="#">0G Explorer ↗</a>
              </div>
            </div>
            <div
              className="contract"
              style={{ gridColumn: "1 / -1", opacity: 0.7 }}
            >
              <div className="ct-name">
                <b>rUSD</b>
                <span className="chip">
                  <span className="d b"></span>Mock token
                </span>
              </div>
              <span className="ct-tag">
                Demo-only ERC-20 stablecoin used for testing · not a production
                asset
              </span>
              <div className="ct-addr">
                <span>0xf0aB · 11D2 · 4cE6 · 8d33 · A218 · b440</span>
                <a href="#">0G Explorer ↗</a>
              </div>
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  );
}

function Stats() {
  return (
    <section className="stats-band">
      <div className="wrap">
        <div className="stats-head">
          <div>
            <span
              className="label label-ink"
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 8,
                marginBottom: 18,
              }}
            >
              § 03 By the numbers
            </span>
            <h2>
              Maximum integrity.
              <br />
              <span style={{ opacity: 0.5 }}>Minimal overhead.</span>
            </h2>
          </div>
          <p className="lead">
            Enterprise-grade payment infrastructure for AI agents, executing
            with sub-second latency on the 0G mainnet.
          </p>
        </div>
        <Reveal className="stats stagger">
          <div className="stat">
            <div className="idx">01 · Netting</div>
            <div className="v">
              <Counter
                from={20}
                to={3}
                duration={1100}
                prefix={
                  <>
                    20<span className="arr">→</span>
                  </>
                }
              />
            </div>
            <div className="k">
              Transactions collapsed into a single batch settlement, on average.
            </div>
          </div>
          <div className="stat">
            <div className="idx">02 · Integrity</div>
            <div className="v">
              <Counter to={100} duration={1100} suffix={<sup>%</sup>} />
            </div>
            <div className="k">
              Of escrow events produce a balanced double-entry journal.
            </div>
          </div>
          <div className="stat">
            <div className="idx">03 · Latency</div>
            <div className="v">
              <Counter to={2} duration={900} prefix="<" suffix={<sup>s</sup>} />
            </div>
            <div className="k">
              Median time from work order creation to funded vault.
            </div>
          </div>
          <div className="stat">
            <div className="idx">04 · Audit</div>
            <div className="v">∞</div>
            <div className="k">
              Permanent audit trail. Every batch anchored to 0G Storage.
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function CTA() {
  return (
    <section className="cta-band">
      <div className="wrap">
        <span
          className="label label-ink"
          style={{ display: "block", marginBottom: 24 }}
        >
          § 04 Get started
        </span>
        <h2>
          Built for the machines
          <br />
          <span className="alt">running the economy.</span>
        </h2>
        <p className="lead">
          Give your agents a financial backbone — escrow, bookkeeping, and
          settlements out of the box. Open spec, open contracts.
        </p>
        <div className="actions">
          <a className="btn btn-primary" href="/dashboard">
            Open the dashboard <span className="btn-arrow">→</span>
          </a>
          <a className="btn btn-ghost" href="#github">
            View on GitHub ↗
          </a>
          <a className="btn btn-ghost" href="#video">
            Watch demo (3:14)
          </a>
        </div>
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer>
      <div className="wrap">
        <div className="foot-grid">
          <div>
            <Logo />
            <p className="foot-tag">
              The settlement layer for AI agents. Escrow, bookkeeping, and
              batched settlements — one protocol.
            </p>
          </div>
          <div>
            <h5>Protocol</h5>
            <a href="#">Specification</a>
            <a href="#">Contracts</a>
            <a href="#">Audit reports</a>
            <a href="#">Roadmap</a>
          </div>
          <div>
            <h5>Build</h5>
            <a href="#">SDK · TypeScript</a>
            <a href="#">SDK · Python</a>
            <a href="#">Examples</a>
            <a href="#">Discord</a>
          </div>
          <div>
            <h5>Network</h5>
            <a href="#">0G Mainnet</a>
            <a href="#">Status page</a>
            <a href="#">Block explorer</a>
            <a href="#">Bridge</a>
          </div>
        </div>
        <div className="foot-bottom">
          <span>RIVE PROTOCOL · 2026</span>
          <span>riveprotocol.tech</span>
        </div>
      </div>
      <div className="foot-mark">rive</div>
    </footer>
  );
}

export default function Home() {
  return (
    <>
      <Nav />
      <Hero />
      <Features />
      <Flow />
      <Stats />
      <CTA />
      <Footer />
    </>
  );
}
