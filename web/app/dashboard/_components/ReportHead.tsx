import type { AgentData } from "./types";
import { shortAddr, fmtDate, fmtTime, relTime } from "./data";
import { Copy } from "./Copy";
import { Icon } from "./icons";

export function ReportHead({ data }: { data: AgentData }) {
  const { agent, period, asset, generatedAt } = data;

  return (
    <div className="db-report-head">
      <div className="db-rh-cell">
        <div className="db-rh-k">Agent address</div>
        <div className="db-rh-v">
          <span className="db-rh-vv">{shortAddr(agent.address)}</span>
          <Copy text={agent.address} />
          <a href="#" className="db-copy" title="View on 0G Explorer" onClick={(e) => e.preventDefault()}>
            <Icon.ext />
          </a>
        </div>
        <div className="db-rh-sub">Registered {fmtDate(agent.registeredAt)}</div>
      </div>
      <div className="db-rh-cell">
        <div className="db-rh-k">Period</div>
        <div className="db-rh-v db-rh-vv">
          {period.from ? fmtDate(period.from) : "All-time"} → {fmtDate(period.to)}
        </div>
        <div className="db-rh-sub">UTC, inclusive</div>
      </div>
      <div className="db-rh-cell">
        <div className="db-rh-k">Asset</div>
        <div className="db-rh-v">
          <span className="db-asset-badge">r$</span>
          <span className="db-rh-vv">{asset}</span>
        </div>
        <div className="db-rh-sub">{data.decimals} decimals</div>
      </div>
      <div className="db-rh-cell">
        <div className="db-rh-k">Generated</div>
        <div className="db-rh-v db-rh-vv">{fmtTime(generatedAt)}</div>
        <div className="db-rh-sub">{relTime(generatedAt)} · v{data.version}</div>
      </div>
    </div>
  );
}
