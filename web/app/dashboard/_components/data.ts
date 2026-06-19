import type { DemoAgent } from "./types";

// ---- Formatters ----

export function fmtAmount(rawStr: string | null | undefined, decimals = 18, opts: { showSign?: boolean; max?: number } = {}): string {
  const { showSign = false, max = 4 } = opts;
  if (rawStr == null) return "—";
  const neg = rawStr.startsWith("-");
  const abs = (neg ? rawStr.slice(1) : rawStr).replace(/^0+(?=\d)/, "") || "0";
  if (!/^\d+$/.test(abs)) return rawStr;
  const padded = abs.padStart(decimals + 1, "0");
  const whole = padded.slice(0, padded.length - decimals);
  const fracRaw = padded.slice(padded.length - decimals);
  const fracStr = fracRaw.slice(0, max).replace(/0+$/, "");
  const wholeFmt = whole.replace(/^0+(?=\d)/, "").replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  let s = fracStr.length ? `${wholeFmt}.${fracStr}` : wholeFmt;
  const isZero = whole.replace(/^0+/, "") === "" && fracStr === "";
  if (neg) s = "-" + s;
  else if (showSign && !isZero) s = "+" + s;
  return s;
}

export function strCmp(a: string, b: string): number {
  a = a.replace(/^0+(?=\d)/, "") || "0";
  b = b.replace(/^0+(?=\d)/, "") || "0";
  if (a.length !== b.length) return a.length < b.length ? -1 : 1;
  return a < b ? -1 : a > b ? 1 : 0;
}

export function strAdd(a: string, b: string): string {
  let i = a.length - 1, j = b.length - 1, carry = 0, out = "";
  while (i >= 0 || j >= 0 || carry) {
    const s = (i >= 0 ? +a[i--] : 0) + (j >= 0 ? +b[j--] : 0) + carry;
    out = (s % 10) + out;
    carry = Math.floor(s / 10);
  }
  return out.replace(/^0+(?=\d)/, "");
}

export function shortAddr(a: string | undefined | null): string {
  if (!a) return "—";
  return a.length > 12 ? `${a.slice(0, 6)}…${a.slice(-4)}` : a;
}

export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleString("en-US", { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleDateString("en-US", { month: "short", day: "2-digit", year: "numeric" });
}

export function relTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

// ---- Demo agents (dashboard quick-fill) ----

export const DEMO_AGENTS: DemoAgent[] = [
  { label: "Inference agent", status: "profitable", address: "0x933a54D5D7a6c0c9e6318395a74cb99Ac1C56934" },
  { label: "Buyer agent",     status: "loss",       address: "0x26Dea28e89DFdF4Cd5ab9f63010Bb46316Ec3a73" },
];
