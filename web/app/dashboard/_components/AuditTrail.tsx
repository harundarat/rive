"use client";

import type { AuditTrailData, AuditBatch } from "./types";
import { fmtTime } from "./data";
import { Icon } from "./icons";

function AuditSourceBadge({ source }: { source: string }) {
  const isNetting = source === "netting_settle" || source === "netting_batch";
  const label = isNetting ? "Netting batch" : "Escrow event";
  const style = isNetting
    ? { background: "rgba(45,210,185,0.14)", color: "#4dd8c8", borderColor: "rgba(45,210,185,0.28)" }
    : { background: "rgba(189,187,255,0.14)", color: "#bdbbff", borderColor: "rgba(189,187,255,0.28)" };
  return (
    <span className="db-source-badge" style={style}>{label}</span>
  );
}

function BatchCard({ batch, index, total }: { batch: AuditBatch; index: number; total: number }) {
  const hasStorageRoot = Boolean(batch.storageRootHash);
  const hasStorageProof = Boolean(batch.storageRootHash && batch.explorerUrl);

  function copyHash() {
    if (!batch.storageRootHash) return;
    navigator.clipboard.writeText(batch.storageRootHash);
    window.dispatchEvent(new CustomEvent("rive-toast", { detail: "Hash copied" }));
  }

  return (
    <div className="db-batch">
      <div style={{ minWidth: 0 }}>
        <div className="db-batch-meta">
          <div>
            <div className="db-batch-k">Batch</div>
            <div className="db-batch-v">#{total - index}</div>
          </div>
          <div>
            <div className="db-batch-k">Source</div>
            <div className="db-batch-v"><AuditSourceBadge source={batch.source} /></div>
          </div>
          <div>
            <div className="db-batch-k">Entries</div>
            <div className="db-batch-v">{batch.entryCount}</div>
          </div>
          <div>
            <div className="db-batch-k">Anchored</div>
            <div className="db-batch-v">{fmtTime(batch.anchoredAt)}</div>
          </div>
        </div>
        <div className="db-hashline">
          <span className="db-hash-label">Storage root</span>
          <span style={{ flex: 1 }}>
            {batch.storageRootHash || "Storage root not available"}
          </span>
          {hasStorageRoot && (
            <span
              className="db-copy"
              style={{ background: "rgba(255,255,255,0.06)", borderColor: "rgba(255,255,255,0.12)", color: "#fff" }}
              onClick={copyHash}
            >
              <Icon.copy />
            </span>
          )}
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 8, alignItems: "stretch" }}>
        {hasStorageProof ? (
          <a className="db-btn db-btn-glass" href={batch.explorerUrl || undefined} target="_blank" rel="noopener noreferrer" style={{ justifyContent: "center" }}>
            Storage proof <Icon.arrow />
          </a>
        ) : (
          <span className="db-btn db-btn-glass db-btn-disabled" style={{ justifyContent: "center" }}>
            Storage proof unavailable
          </span>
        )}
        {batch.chainExplorerUrl && (
          <a className="db-btn db-btn-glass" href={batch.chainExplorerUrl} target="_blank" rel="noopener noreferrer" style={{ justifyContent: "center" }}>
            Settlement tx <Icon.arrow />
          </a>
        )}
      </div>
    </div>
  );
}

export function AuditTrail({ trail }: { trail: AuditTrailData }) {
  const isEmpty = !trail.batches || trail.batches.length === 0;
  const totalEntries = trail.batches.reduce((s, b) => s + b.entryCount, 0);

  return (
    <section className="db-audit">
      <div className="db-section-label" style={{ color: "rgba(255,255,255,0.55)" }}>
        On-chain verification · 0G Storage
      </div>
      <h2 className="db-audit-h2">Audit trail</h2>
      <p className="db-audit-lede">
        Every journal batch is hashed and anchored to 0G Storage. Anyone can independently verify
        that the books shown above were not modified after publication.
      </p>

      {isEmpty ? (
        <div className="db-audit-empty">
          <div style={{ fontSize: 16, fontWeight: 500, marginBottom: 8 }}>No journal batches anchored yet</div>
          <div style={{ fontSize: 14, color: "rgba(255,255,255,0.50)" }}>
            Batches will appear here once escrow or netting events are recorded on-chain.
          </div>
        </div>
      ) : (
        <>
          <div className="db-audit-summary">
            <div>
              <div className="db-audit-k">Batches</div>
              <div className="db-audit-num">{trail.journalBatchCount}</div>
            </div>
            <div>
              <div className="db-audit-k">Total entries anchored</div>
              <div className="db-audit-num">{totalEntries}</div>
            </div>
            <div>
              <div className="db-audit-k">Verifier</div>
              <div className="db-audit-verifier">
                <Icon.shield /> storagescan.0g.ai
              </div>
            </div>
          </div>
          <div>
            {trail.batches.map((b, i) => (
              <BatchCard key={b.batchId} batch={b} index={i} total={trail.batches.length} />
            ))}
          </div>
        </>
      )}
    </section>
  );
}
