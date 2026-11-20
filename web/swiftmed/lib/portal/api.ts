// Server-side client for the pharmacy portal's REST Gateway calls (SCRUM-139:
// moved here from the former pharmacy-dashboard app).

/** What the portal needs from the signed-in staff session (lib/portal/data.ts). */
export interface PortalSession {
  pharmacyId: string;
  accessToken: string;
  email?: string;
}

export const GATEWAY_URL = (process.env.GATEWAY_URL ?? "http://localhost:8080").replace(/\/+$/, "");
const REQUEST_TIMEOUT_MS = 5_000;

export interface StockLevel {
  pharmacyId: string;
  productId: string;
  quantityOnHand?: number;
  quantityReserved?: number;
  quantityAvailable?: number;
  confirmedAt?: string;
}

export interface StockListResponse {
  stock?: StockLevel[];
  page?: {
    nextPageToken?: string;
  };
}

export interface Hold {
  holdId: string;
  userId?: string;
  pharmacyId?: string;
  productId?: string;
  quantity?: number;
  status: string;
  createdAt?: string;
  expiresAt?: string;
  statusChangedAt?: string;
  statusReason?: string;
  // Pharmacy staff do not receive this field in the list response.
  pickupCode?: string;
}

export interface UnmetDemandItem {
  productId: string;
  productName: string;
  searchCount?: number;
  unmetCount?: number;
}

export interface UnmetDemandResponse {
  items?: UnmetDemandItem[];
}

export interface HoldListResponse {
  holds?: Hold[];
  page?: {
    nextPageToken?: string;
  };
}

export type SyncJobStatus =
  | "SYNC_JOB_STATUS_PROCESSING"
  | "SYNC_JOB_STATUS_COMPLETED"
  | "SYNC_JOB_STATUS_PARTIAL"
  | "SYNC_JOB_STATUS_FAILED";

export interface RowError {
  /** 1-based, counting the header as row 1. */
  rowNumber?: number;
  productId?: string;
  message?: string;
}

export interface SyncJob {
  jobId: string;
  pharmacyId?: string;
  uploadedByUserId?: string;
  filename?: string;
  status: SyncJobStatus | string;
  rowsTotal?: number;
  rowsApplied?: number;
  rowsRejected?: number;
  rowErrors?: RowError[];
  createdAt?: string;
  finishedAt?: string;
}

export class DashboardApiError extends Error {
  constructor(
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = "DashboardApiError";
  }
}

function errorMessageFromBody(value: unknown): string | undefined {
  if (typeof value === "string" && value.trim()) return value.trim();
  if (typeof value !== "object" || value === null) return undefined;

  const body = value as Record<string, unknown>;
  if (typeof body.message === "string" && body.message.trim()) return body.message.trim();
  if (typeof body.detail === "string" && body.detail.trim()) return body.detail.trim();

  if (typeof body.error === "string" && body.error.trim()) return body.error.trim();
  if (typeof body.error === "object" && body.error !== null) {
    const error = body.error as Record<string, unknown>;
    if (typeof error.message === "string" && error.message.trim()) return error.message.trim();
    if (typeof error.detail === "string" && error.detail.trim()) return error.detail.trim();
  }

  return undefined;
}

function fallbackErrorMessage(status: number): string {
  switch (status) {
    case 400:
      return "The request was invalid. Check the values and try again.";
    case 401:
      return "Your session has expired. Please log in again.";
    case 403:
      return "Your account cannot access this pharmacy action.";
    case 404:
      return "The requested stock item or hold was not found.";
    case 409:
    case 412:
      return "The hold has changed. Refresh the dashboard and try again.";
    case 413:
      return "The file is larger than 1 MB.";
    default:
      return status >= 500
        ? "The Gateway could not complete the request."
        : `The request failed (HTTP ${status}).`;
  }
}

async function requestJSON<T>(
  method: "GET" | "POST" | "PUT",
  path: string,
  options: {
    session?: PortalSession;
    body?: unknown;
  } = {},
): Promise<T> {
  const headers = new Headers({ Accept: "application/json" });

  if (options.session) {
    headers.set("Authorization", `Bearer ${options.session.accessToken}`);
  }
  // FormData sets its own multipart Content-Type with the boundary.
  const isForm = options.body instanceof FormData;
  if (options.body !== undefined && !isForm) {
    headers.set("Content-Type", "application/json");
  }

  let response: Response;
  try {
    response = await fetch(new URL(path, `${GATEWAY_URL}/`), {
      method,
      headers,
      body:
        options.body === undefined
          ? undefined
          : isForm
            ? (options.body as FormData)
            : JSON.stringify(options.body),
      cache: "no-store",
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch (cause) {
    const error = cause as { name?: unknown } | null;
    if (error?.name === "TimeoutError" || error?.name === "AbortError") {
      throw new DashboardApiError("The Gateway did not answer in time.");
    }
    throw new DashboardApiError("Could not reach the API Gateway.");
  }

  const responseText = await response.text();
  let responseBody: unknown;

  if (responseText) {
    try {
      responseBody = JSON.parse(responseText);
    } catch {
      if (response.ok) {
        throw new DashboardApiError("The Gateway returned an invalid response.", response.status);
      }
    }
  }

  if (!response.ok) {
    // Use the Gateway's validation message when available, especially for a
    // wrong pickup code. Fall back to a clear status message otherwise.
    const message =
      response.status < 500
        ? errorMessageFromBody(responseBody) ?? fallbackErrorMessage(response.status)
        : fallbackErrorMessage(response.status);
    throw new DashboardApiError(message, response.status);
  }

  return responseBody as T;
}

export async function listPharmacyStock(
  session: PortalSession,
  pageToken?: string,
): Promise<StockLevel[]> {
  const query = new URLSearchParams({ pageSize: "50" });
  if (pageToken) query.set("pageToken", pageToken);

  const result = await requestJSON<StockListResponse>(
    "GET",
    `/api/v1/pharmacies/${encodeURIComponent(session.pharmacyId)}/stock?${query.toString()}`,
    { session },
  );

  return Array.isArray(result.stock) ? result.stock : [];
}

export function setStock(
  session: PortalSession,
  productId: string,
  quantityOnHand: number,
): Promise<StockLevel> {
  return requestJSON<StockLevel>(
    "PUT",
    `/api/v1/pharmacies/${encodeURIComponent(session.pharmacyId)}/stock/${encodeURIComponent(productId)}`,
    {
      session,
      body: { quantityOnHand },
    },
  );
}

export async function listActiveHolds(session: PortalSession): Promise<Hold[]> {
  const query = new URLSearchParams({
    status: "HOLD_STATUS_ACTIVE",
    pageSize: "50",
  });

  const result = await requestJSON<HoldListResponse>(`GET`, `/api/v1/holds?${query.toString()}`, { session });
  return Array.isArray(result.holds) ? result.holds : [];
}

export async function getUnmetDemand(session: PortalSession,): Promise<UnmetDemandItem[]> {
  const result = await requestJSON<UnmetDemandResponse>(
    "GET",
    "/api/v1/pharmacies/me/analytics/unmet-demand",
    { session },
  );

  return Array.isArray(result.items) ? result.items.slice(0, 10) : [];
}

function stockSyncPath(session: PortalSession): string {
  return `/api/v1/pharmacies/${encodeURIComponent(session.pharmacyId)}/stock-sync/jobs`;
}

/** Sends a stock CSV to Stock Sync. The returned job holds the per-row report. */
export function uploadStockCsv(session: PortalSession, file: File): Promise<SyncJob> {
  const body = new FormData();
  body.set("file", file, file.name);
  return requestJSON<SyncJob>("POST", stockSyncPath(session), { session, body });
}

export function getSyncJob(session: PortalSession, jobId: string): Promise<SyncJob> {
  return requestJSON<SyncJob>("GET", `${stockSyncPath(session)}/${encodeURIComponent(jobId)}`, {
    session,
  });
}

export function collectHold(
  session: PortalSession,
  holdId: string,
  pickupCode: string,
): Promise<Hold> {
  return requestJSON<Hold>(
    "POST",
    `/api/v1/holds/${encodeURIComponent(holdId)}/collect`,
    {
      session,
      body: { pickupCode },
    },
  );
}