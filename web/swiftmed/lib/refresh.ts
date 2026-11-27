// Keeps a patient signed in past the 15-minute access token (SCRUM-131).
// POST /api/v1/auth/refresh (docs/api/openapi.yaml) swaps the refresh token for
// a new pair, and the old refresh token stops working at once. Server-side only.
//
// Where it runs: proxy.ts refreshes shortly before the access token expires,
// before any page, route handler or server action sees the request, because
// server components cannot set cookies. Server actions also refresh once after
// a 401 (lib/auth.ts withFreshToken). Both go through refreshOnce, so parallel
// requests with the same cookie share one gateway call instead of racing to use
// a refresh token that only works once.
import { HoldError } from "./holds-core";
import { sharedRefresh, storeFromEnv, type RefreshStore } from "./refresh-store";
import { claimsFromToken } from "./roles";
import { DEFAULT_SESSION_SECONDS } from "./session";
import type { Session } from "./types";

export interface RefreshOptions {
  gatewayUrl: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
  now?: () => number;
  /**
   * Shares refreshes between server instances (SCRUM-160). Undefined: from the
   * environment (KV_REST_API_URL and KV_REST_API_TOKEN); null: none.
   */
  store?: RefreshStore | null;
}

export type RefreshOutcome =
  /** New tokens: save them. */
  | { kind: "refreshed"; session: Session }
  /** The gateway refused the refresh token (invalid, expired, used or revoked): sign out. */
  | { kind: "rejected"; status: number }
  /** The gateway could not be asked (down, slow, 5xx): keep the session and try again on the next request. */
  | { kind: "failed"; reason: string };

/** The token pair of a login or refresh answer (openapi.yaml TokenPair) as session fields. */
export function tokenFields(
  tokens: { accessToken?: unknown; refreshToken?: unknown; accessTokenExpiresAt?: unknown } | null | undefined,
  now = Date.now(),
): Pick<Session, "accessToken" | "expiresAt" | "refreshToken"> | undefined {
  if (typeof tokens?.accessToken !== "string" || !tokens.accessToken) return undefined;
  const expiresAt =
    typeof tokens.accessTokenExpiresAt === "string" && !Number.isNaN(Date.parse(tokens.accessTokenExpiresAt))
      ? tokens.accessTokenExpiresAt
      : // Without an expiry the app cannot know when to refresh, so assume the gateway's default.
        new Date(now + DEFAULT_SESSION_SECONDS * 1000).toISOString();
  const out: Pick<Session, "accessToken" | "expiresAt" | "refreshToken"> = { accessToken: tokens.accessToken, expiresAt };
  if (typeof tokens.refreshToken === "string" && tokens.refreshToken) out.refreshToken = tokens.refreshToken;
  return out;
}

/** One call to the gateway refresh route. Prefer refreshOnce, which shares concurrent calls. */
export async function refreshSession(session: Session, options: RefreshOptions): Promise<RefreshOutcome> {
  if (!session.refreshToken) return { kind: "rejected", status: 0 };
  const fetchFn = options.fetchFn ?? globalThis.fetch;
  const now = options.now ?? Date.now;

  let res: Response;
  try {
    res = await fetchFn(new URL("/api/v1/auth/refresh", options.gatewayUrl).toString(), {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: session.refreshToken }),
      cache: "no-store",
      signal: AbortSignal.timeout(options.timeoutMs ?? 5000),
    });
  } catch (cause) {
    const name = (cause as { name?: unknown } | null)?.name;
    return { kind: "failed", reason: name === "TimeoutError" || name === "AbortError" ? "timeout" : "unreachable" };
  }
  // 400 and 401 are the documented refusals. Any other 4xx (no such route yet,
  // or the route needs a token) will not get better by retrying either.
  if (res.status >= 400 && res.status < 500) return { kind: "rejected", status: res.status };
  if (!res.ok) return { kind: "failed", reason: `HTTP ${res.status}` };

  let body: { tokens?: Parameters<typeof tokenFields>[0] } | null;
  try {
    body = (await res.json()) as typeof body;
  } catch {
    return { kind: "failed", reason: "answer is not JSON" };
  }
  const tokens = tokenFields(body?.tokens, now());
  if (!tokens) return { kind: "failed", reason: "answer has no access token" };
  // The gateway should always send a new refresh token; if it does not, the old
  // one is spent, so the patient signs in again when this access token expires.
  const next: Session = { ...session, ...tokens };
  if (!tokens.refreshToken) delete next.refreshToken;
  // Registering a pharmacy turns a patient account into a staff account
  // (SCRUM-141): the new token says so, and the session follows it.
  const claims = claimsFromToken(tokens.accessToken);
  if (claims.role) next.role = claims.role;
  if (claims.pharmacyId) next.pharmacyId = claims.pharmacyId;
  return { kind: "refreshed", session: next };
}

// ---------------------------------------------------------------------------
// Sharing concurrent refreshes

/**
 * How long a finished refresh is remembered. A browser often sends several
 * requests with the old cookie at once (a page, its alert stream, a prefetch);
 * the ones that arrive after the first refresh finished get the same new tokens
 * instead of sending the spent refresh token and being signed out.
 */
export const REFRESH_REUSE_MS = 30_000;

interface Entry {
  outcome: Promise<RefreshOutcome>;
  /** Set when the outcome is known; the entry is dropped REFRESH_REUSE_MS later. */
  settledAt?: number;
}

// On globalThis so proxy.ts and server actions (bundled separately) share it
// within one server process. Between instances (Vercel), the shared store in
// lib/refresh-store.ts does the same, when it is configured (SCRUM-160).
const memory = globalThis as typeof globalThis & { __swiftmedRefreshes?: Map<string, Entry> };

function entries(): Map<string, Entry> {
  memory.__swiftmedRefreshes ??= new Map();
  return memory.__swiftmedRefreshes;
}

/** One refresh across all instances when a shared store is set up, else this process's own. */
function refreshAcrossInstances(session: Session, options: RefreshOptions): Promise<RefreshOutcome> {
  const shared = options.store === undefined ? storeFromEnv() : (options.store ?? undefined);
  if (!shared || !session.refreshToken) return refreshSession(session, options);
  return sharedRefresh(session.refreshToken, shared, () => refreshSession(session, options));
}

/** refreshSession, shared by every caller holding the same refresh token. Only successes are remembered. */
export function refreshOnce(session: Session, options: RefreshOptions): Promise<RefreshOutcome> {
  const key = session.refreshToken;
  if (!key) return Promise.resolve({ kind: "rejected", status: 0 });
  const now = options.now ?? Date.now;
  const map = entries();

  for (const [k, e] of map) {
    if (e.settledAt !== undefined && now() - e.settledAt > REFRESH_REUSE_MS) map.delete(k);
  }
  const existing = map.get(key);
  if (existing) return existing.outcome;

  const entry: Entry = {
    outcome: refreshAcrossInstances(session, options).then((outcome) => {
      if (outcome.kind === "refreshed") entry.settledAt = now();
      // A refusal or a failure is not kept: the next request may try again.
      else map.delete(key);
      return outcome;
    }),
  };
  map.set(key, entry);
  return entry.outcome;
}

/**
 * Runs a gateway call; after a 401 ("unauthorized") it refreshes once, saves
 * the new session and runs the call again. If there is no refresh token, no
 * gateway, or the refresh does not succeed, the original error is thrown so the
 * caller signs the patient out. A second 401 is thrown as is (no loop).
 */
export async function retryAfterRefresh<T>(
  session: Session,
  run: (session: Session) => Promise<T>,
  options: RefreshOptions & { save: (session: Session) => Promise<void> },
): Promise<T> {
  try {
    return await run(session);
  } catch (err) {
    if (!(err instanceof HoldError && err.kind === "unauthorized") || !session.refreshToken || !options.gatewayUrl) throw err;
    const outcome = await refreshOnce(session, options);
    if (outcome.kind !== "refreshed") throw err;
    await options.save(outcome.session);
    return run(outcome.session);
  }
}

/** For tests. */
export function forgetRefreshes(): void {
  entries().clear();
}

/** Best effort POST /api/v1/auth/logout, so a signed-out refresh token cannot be used again. Never throws. */
export async function revokeRefreshToken(refreshToken: string, options: RefreshOptions): Promise<void> {
  try {
    await (options.fetchFn ?? globalThis.fetch)(new URL("/api/v1/auth/logout", options.gatewayUrl).toString(), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken }),
      cache: "no-store",
      signal: AbortSignal.timeout(options.timeoutMs ?? 2000),
    });
  } catch {
    // Signing out locally still works; the token expires on its own.
  }
}
