// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

import { forgetRefreshes, refreshOnce } from "./refresh";
import {
  openSession,
  sealSession,
  sharedRefresh,
  storeFromEnv,
  storeKey,
  upstashStore,
  type RefreshStore,
  type SharedOutcome,
} from "./refresh-store";
import type { Session } from "./types";

/** An in-memory store with the same semantics as Redis' SET NX EX. */
function memoryStore(): RefreshStore & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    get: async (k) => data.get(k) ?? null,
    setIfAbsent: async (k, v) => {
      if (data.has(k)) return false;
      data.set(k, v);
      return true;
    },
    set: async (k, v) => {
      data.set(k, v);
    },
    del: async (k) => {
      data.delete(k);
    },
  };
}

const old: Session = { userId: "u-1", email: "tashi@example.bt", accessToken: "old-access", refreshToken: "old-refresh" };
const fresh: Session = { ...old, accessToken: "new-access", refreshToken: "new-refresh", expiresAt: "2026-11-26T10:15:00Z" };
const noSleep = () => Promise.resolve();

afterEach(() => forgetRefreshes());

describe("sharedRefresh (SCRUM-160)", () => {
  it("refreshes once across instances: the second reuses the first's result", async () => {
    const store = memoryStore();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const refresh = vi.fn(async (): Promise<SharedOutcome> => {
      await gate;
      return { kind: "refreshed", session: fresh };
    });
    // Two instances with the same refresh token at the same time.
    const first = sharedRefresh("old-refresh", store, refresh, { sleep: noSleep, pollMs: 1 });
    const second = sharedRefresh("old-refresh", store, refresh, { sleep: () => new Promise((r) => setTimeout(r, 5)), pollMs: 5 });
    await new Promise((r) => setTimeout(r, 10));
    release();
    expect(await first).toEqual({ kind: "refreshed", session: fresh });
    expect(await second).toEqual({ kind: "refreshed", session: fresh });
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("gives a late request with the old token the same new tokens", async () => {
    const store = memoryStore();
    const refresh = vi.fn(async (): Promise<SharedOutcome> => ({ kind: "refreshed", session: fresh }));
    await sharedRefresh("old-refresh", store, refresh);
    expect(await sharedRefresh("old-refresh", store, refresh)).toEqual({ kind: "refreshed", session: fresh });
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("stores neither the refresh token nor readable tokens", async () => {
    const store = memoryStore();
    await sharedRefresh("old-refresh", store, async () => ({ kind: "refreshed", session: fresh }));
    const everything = [...store.data.keys(), ...store.data.values()].join(" ");
    for (const secret of ["old-refresh", "new-refresh", "new-access", "tashi@example.bt"]) expect(everything).not.toContain(secret);
    const sealed = store.data.get(await storeKey("old-refresh"))!;
    expect(await openSession(sealed, "old-refresh")).toEqual(fresh);
    expect(await openSession(sealed, "someone-else")).toBeUndefined();
  });

  it("does not keep a refusal, and the waiting instance keeps its session instead of signing out", async () => {
    const store = memoryStore();
    expect(await sharedRefresh("old-refresh", store, async () => ({ kind: "rejected", status: 401 }))).toEqual({ kind: "rejected", status: 401 });
    expect(store.data.size).toBe(0);

    // Another instance holds the lock and never finishes.
    await store.setIfAbsent(`${await storeKey("other")}:lock`, "1", 10);
    const refresh = vi.fn();
    const outcome = await sharedRefresh("other", store, refresh, { sleep: noSleep, waitMs: 30, pollMs: 10 });
    expect(outcome.kind).toBe("failed");
    expect(refresh).not.toHaveBeenCalled();
  });

  it("refreshes on its own when the store is down", async () => {
    const broken: RefreshStore = {
      get: () => Promise.reject(new Error("down")),
      setIfAbsent: () => Promise.reject(new Error("down")),
      set: () => Promise.reject(new Error("down")),
      del: () => Promise.reject(new Error("down")),
    };
    expect(await sharedRefresh("old-refresh", broken, async () => ({ kind: "refreshed", session: fresh }))).toEqual({
      kind: "refreshed",
      session: fresh,
    });
  });
});

describe("the Upstash store", () => {
  it("is configured from Vercel's or Upstash's variables, and is off without them", () => {
    expect(storeFromEnv({})).toBeUndefined();
    expect(storeFromEnv({ KV_REST_API_URL: "https://x.upstash.io" })).toBeUndefined();
    expect(storeFromEnv({ KV_REST_API_URL: "https://x.upstash.io", KV_REST_API_TOKEN: "t" })).toBeDefined();
    expect(storeFromEnv({ UPSTASH_REDIS_REST_URL: "https://x.upstash.io", UPSTASH_REDIS_REST_TOKEN: "t" })).toBeDefined();
  });

  it("sends Redis commands to the REST API", async () => {
    const fetchFn = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      const cmd = JSON.parse(String(init?.body)) as string[];
      const result = cmd[0] === "SET" ? "OK" : cmd[0] === "GET" ? "value" : 1;
      return new Response(JSON.stringify({ result }), { status: 200 });
    });
    const store = upstashStore("https://x.upstash.io/", "secret-token", fetchFn as unknown as typeof fetch);
    expect(await store.setIfAbsent("k", "v", 10)).toBe(true);
    expect(await store.get("k")).toBe("value");
    const [url, init] = fetchFn.mock.calls[0];
    expect(String(url)).toBe("https://x.upstash.io");
    expect(new Headers(init?.headers).get("authorization")).toBe("Bearer secret-token");
    expect(JSON.parse(String(init?.body))).toEqual(["SET", "k", "v", "NX", "EX", "10"]);
  });
});

describe("refreshOnce with a shared store", () => {
  it("uses the store's result instead of calling the gateway", async () => {
    const store = memoryStore();
    await store.set(await storeKey("old-refresh"), await sealSession(fresh, "old-refresh"), 30);
    const fetchFn = vi.fn();
    const outcome = await refreshOnce(old, { gatewayUrl: "http://gateway:8080", fetchFn: fetchFn as unknown as typeof fetch, store });
    expect(outcome).toEqual({ kind: "refreshed", session: fresh });
    expect(fetchFn).not.toHaveBeenCalled();
  });
});
