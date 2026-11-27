// Sharing token refreshes between server instances (SCRUM-160).
//
// A refresh token works once. lib/refresh.ts shares one refresh between the
// requests of one server process; on Vercel, requests run on separate
// instances, and two of them refreshing the same token at once sign the user
// out (the second refresh is refused). With a shared store (Upstash Redis over
// its REST API, as Vercel's marketplace provides), one instance refreshes under
// a short lock and the others reuse its result.
//
// The store never holds anything usable: the result is encrypted with a key
// derived from the old refresh token, so only a request that already holds that
// token can read it, and the lookup key is a separate hash of it.
import type { Session } from "./types";

export interface RefreshStore {
  get(key: string): Promise<string | null>;
  /** Sets the key only if it is absent; true when this call set it. */
  setIfAbsent(key: string, value: string, ttlSeconds: number): Promise<boolean>;
  set(key: string, value: string, ttlSeconds: number): Promise<void>;
  del(key: string): Promise<void>;
}

export type SharedOutcome =
  | { kind: "refreshed"; session: Session }
  | { kind: "rejected"; status: number }
  | { kind: "failed"; reason: string };

type Env = Readonly<Record<string, string | undefined>>;
type FetchFn = typeof fetch;

/** Upstash Redis over its REST API: one POST per command, ["SET", key, value, ...]. */
export function upstashStore(url: string, token: string, fetchFn: FetchFn = fetch, timeoutMs = 2000): RefreshStore {
  const base = url.replace(/\/+$/, "");
  async function command(args: (string | number)[]): Promise<unknown> {
    const res = await fetchFn(base, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify(args.map(String)),
      cache: "no-store",
      signal: AbortSignal.timeout(timeoutMs),
    });
    if (!res.ok) throw new Error(`refresh store answered HTTP ${res.status}`);
    const body = (await res.json()) as { result?: unknown; error?: unknown };
    if (body.error) throw new Error(`refresh store: ${String(body.error)}`);
    return body.result ?? null;
  }
  return {
    get: async (key) => {
      const v = await command(["GET", key]);
      return typeof v === "string" ? v : null;
    },
    setIfAbsent: async (key, value, ttl) => (await command(["SET", key, value, "NX", "EX", ttl])) === "OK",
    set: async (key, value, ttl) => {
      await command(["SET", key, value, "EX", ttl]);
    },
    del: async (key) => {
      await command(["DEL", key]);
    },
  };
}

/**
 * The shared store from the environment, or undefined (then refreshes are
 * shared within one process only, as before). Vercel's Upstash integration sets
 * KV_REST_API_URL and KV_REST_API_TOKEN; Upstash's own names work too.
 */
export function storeFromEnv(env: Env = process.env, fetchFn: FetchFn = fetch): RefreshStore | undefined {
  const url = env.KV_REST_API_URL || env.UPSTASH_REDIS_REST_URL;
  const token = env.KV_REST_API_TOKEN || env.UPSTASH_REDIS_REST_TOKEN;
  if (!url?.trim() || !token?.trim()) return undefined;
  return upstashStore(url.trim(), token.trim(), fetchFn);
}

// ---------------------------------------------------------------------------
// Keys and encryption (Web Crypto, available in Node and edge runtimes)

const enc = new TextEncoder();
const dec = new TextDecoder();
const toHex = (b: ArrayBuffer) => [...new Uint8Array(b)].map((x) => x.toString(16).padStart(2, "0")).join("");
const toB64 = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const fromB64 = (s: string) => new Uint8Array(Buffer.from(s, "base64url"));

async function sha256(text: string): Promise<ArrayBuffer> {
  return crypto.subtle.digest("SHA-256", enc.encode(text));
}

/** The store key for a refresh token: a hash, so the token itself is never stored. */
export async function storeKey(refreshToken: string): Promise<string> {
  return `swiftmed:refresh:${toHex(await sha256(`swiftmed-refresh-id:${refreshToken}`))}`;
}

async function cipherKey(refreshToken: string): Promise<CryptoKey> {
  return crypto.subtle.importKey("raw", await sha256(`swiftmed-refresh-key:${refreshToken}`), "AES-GCM", false, ["encrypt", "decrypt"]);
}

export async function sealSession(session: Session, refreshToken: string): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const data = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, await cipherKey(refreshToken), enc.encode(JSON.stringify(session)));
  return `${toB64(iv)}.${toB64(new Uint8Array(data))}`;
}

export async function openSession(sealed: string, refreshToken: string): Promise<Session | undefined> {
  const [iv, data] = sealed.split(".");
  if (!iv || !data) return undefined;
  try {
    const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: fromB64(iv) }, await cipherKey(refreshToken), fromB64(data));
    return JSON.parse(dec.decode(plain)) as Session;
  } catch {
    return undefined;
  }
}

// ---------------------------------------------------------------------------

export interface SharedOptions {
  /** How long a refresh may hold the lock. */
  lockSeconds?: number;
  /** How long the result is kept for late requests with the old cookie. */
  resultSeconds?: number;
  /** How long another instance waits for the result. */
  waitMs?: number;
  pollMs?: number;
  sleep?: (ms: number) => Promise<void>;
}

/**
 * Runs `refresh` once across all instances for this refresh token. Another
 * instance's result is reused; while one is running, the others wait for it.
 * If the store fails, this instance refreshes on its own (the old behaviour).
 * If waiting runs out, the outcome is "failed": the request keeps its session
 * and the next one tries again, rather than signing the user out.
 */
export async function sharedRefresh(
  refreshToken: string,
  store: RefreshStore,
  refresh: () => Promise<SharedOutcome>,
  options: SharedOptions = {},
): Promise<SharedOutcome> {
  const { lockSeconds = 10, resultSeconds = 30, waitMs = 5000, pollMs = 150 } = options;
  const sleep = options.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));

  let key: string;
  let locked: boolean;
  try {
    key = await storeKey(refreshToken);
    const cached = await store.get(key);
    const reused = cached ? await openSession(cached, refreshToken) : undefined;
    if (reused) return { kind: "refreshed", session: reused };
    locked = await store.setIfAbsent(`${key}:lock`, "1", lockSeconds);
  } catch {
    return refresh(); // the store is down: refresh here, as without a store
  }

  if (locked) {
    const outcome = await refresh();
    try {
      if (outcome.kind === "refreshed") await store.set(key, await sealSession(outcome.session, refreshToken), resultSeconds);
      await store.del(`${key}:lock`);
    } catch {
      // The refresh worked; only sharing it failed.
    }
    return outcome;
  }

  // Another instance is refreshing this token: wait for its result.
  for (let waited = 0; waited < waitMs; waited += pollMs) {
    await sleep(pollMs);
    try {
      const cached = await store.get(key);
      const reused = cached ? await openSession(cached, refreshToken) : undefined;
      if (reused) return { kind: "refreshed", session: reused };
      if (!(await store.get(`${key}:lock`))) break; // it finished without a result (refused or failed)
    } catch {
      break;
    }
  }
  return { kind: "failed", reason: "another server is refreshing this session" };
}
