/** Settings the Worker needs. See wrangler.toml for where each one comes from. */
export interface Env {
  /** Base URL of the gateway, for example https://api.example.com (no path). */
  GATEWAY_URL: string;
  /** Shared secret the gateway checks. Never log it. */
  WORKER_SHARED_SECRET: string;
}

/** The gateway endpoint that asks Expiry to expire overdue holds (docs/api/openapi.yaml). */
export const SWEEP_PATH = "/api/v1/internal/expiry/sweep";

/** The header that carries the shared secret. */
export const SECRET_HEADER = "X-Worker-Secret";

/** Give up on a call after this long. It stays well under the one-minute schedule. */
export const REQUEST_TIMEOUT_MS = 25_000;

type FetchFn = typeof fetch;

/** Things tests replace. In production the defaults are used. */
export interface Deps {
  fetchFn?: FetchFn;
  now?: () => Date;
}

// Looked up at call time, so tests can replace the global fetch.
const defaultFetch: FetchFn = (input, init) => fetch(input, init);

/**
 * Calls the gateway once and returns the IDs of the holds that were expired.
 * Sends the current time as `asOf`, as the API contract asks. Throws if settings
 * are missing, the gateway is unreachable, or it answers with anything other
 * than a 2xx JSON object.
 */
export async function callSweep(env: Env, deps: Deps = {}): Promise<string[]> {
  const fetchFn = deps.fetchFn ?? defaultFetch;
  const now = deps.now ?? (() => new Date());

  const baseUrl = (env.GATEWAY_URL ?? "").trim().replace(/\/+$/, "");
  const secret = (env.WORKER_SHARED_SECRET ?? "").trim();
  if (baseUrl === "") {
    throw new Error("GATEWAY_URL is not set");
  }
  if (secret === "") {
    throw new Error("WORKER_SHARED_SECRET is not set");
  }

  const response = await fetchFn(`${baseUrl}${SWEEP_PATH}`, {
    method: "POST",
    headers: {
      [SECRET_HEADER]: secret,
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ asOf: now().toISOString() }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });

  if (!response.ok) {
    // Read and drop the body so the connection is freed. It is not logged.
    await response.text().catch(() => "");
    throw new Error(`gateway returned HTTP ${response.status}`);
  }

  const body: unknown = await response.json().catch(() => null);
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    throw new Error("gateway response is not a JSON object");
  }
  // Protobuf JSON leaves out an empty list, so a missing expiredHoldIds means none expired.
  const ids = (body as { expiredHoldIds?: unknown }).expiredHoldIds;
  if (ids === undefined) {
    return [];
  }
  if (!Array.isArray(ids) || !ids.every((id) => typeof id === "string")) {
    throw new Error("gateway response has an invalid expiredHoldIds");
  }
  return ids as string[];
}
