import type { AgentResponse } from "./types";

export const DEFAULT_WALLET_ADDRESS =
  "0x26Dea28e89DFdF4Cd5ab9f63010Bb46316Ec3a73";

const DEFAULT_API_BASE_URL = "http://localhost:8080";

interface APIErrorBody {
  code?: string;
  message?: string;
}

interface APIErrorEnvelope {
  success: false;
  error?: APIErrorBody;
}

export class PnLApiError extends Error {
  code: string;
  status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "PnLApiError";
    this.code = code;
    this.status = status;
  }
}

function apiBaseURL(): string {
  const configured = process.env.NEXT_PUBLIC_RIVE_API_BASE_URL?.trim();
  return (configured || DEFAULT_API_BASE_URL).replace(/\/+$/, "");
}

function pnlURL(walletAddress: string): string {
  return `${apiBaseURL()}/api/ledger/${encodeURIComponent(walletAddress)}/pnl`;
}

function isAPIErrorEnvelope(value: unknown): value is APIErrorEnvelope {
  return (
    typeof value === "object" &&
    value !== null &&
    "success" in value &&
    (value as { success?: unknown }).success === false
  );
}

function isAgentResponse(value: unknown): value is AgentResponse {
  return (
    typeof value === "object" &&
    value !== null &&
    (value as { success?: unknown }).success === true &&
    typeof (value as { data?: unknown }).data === "object" &&
    (value as { data?: unknown }).data !== null
  );
}

export async function fetchPnLReport(
  walletAddress: string,
  signal?: AbortSignal,
): Promise<AgentResponse> {
  const trimmed = walletAddress.trim();
  if (!trimmed) {
    throw new PnLApiError("Wallet address is required.", "BAD_REQUEST", 400);
  }

  const response = await fetch(pnlURL(trimmed), {
    method: "GET",
    cache: "no-store",
    headers: { Accept: "application/json" },
    signal,
  });

  let body: unknown = null;
  try {
    body = await response.json();
  } catch {
    if (!response.ok) {
      throw new PnLApiError(
        `Failed to fetch P&L report (${response.status}).`,
        "HTTP_ERROR",
        response.status,
      );
    }
  }

  if (!response.ok) {
    if (isAPIErrorEnvelope(body)) {
      throw new PnLApiError(
        body.error?.message || `Failed to fetch P&L report (${response.status}).`,
        body.error?.code || "HTTP_ERROR",
        response.status,
      );
    }

    throw new PnLApiError(
      `Failed to fetch P&L report (${response.status}).`,
      "HTTP_ERROR",
      response.status,
    );
  }

  if (!isAgentResponse(body)) {
    throw new PnLApiError(
      "Backend returned an unexpected P&L response.",
      "INVALID_RESPONSE",
      response.status,
    );
  }

  return body;
}
