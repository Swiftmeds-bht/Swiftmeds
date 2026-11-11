import { describe, expect, it, vi } from "vitest";

import { DEMO_ACCOUNT, demoBackend } from "./demo-holds";
import { HoldError } from "./holds";
import { MOCK_PRODUCTS } from "./mock-data";
import type { StockAlert } from "./types";
import { DEMO_RESTOCK_MS, DemoWatchlist, gatewayWatchlist, watchlistBackend } from "./watchlist";

const session = { userId: "u-1", email: "tashi@example.bt", accessToken: "tok-1" };
const thimphu = { latitude: 27.4728, longitude: 89.639 };
const input = { productId: "p-1", location: thimphu, radiusM: 5000 };
const sub = { subscriptionId: "s-1", productId: "p-1", location: thimphu, radiusM: 5000, createdAt: "2026-11-11T08:00:00Z" };
const alert: StockAlert = { subscriptionId: "s-1", productId: "p-1", pharmacyId: "ph-1", quantityAvailable: 4, occurredAt: "2026-11-11T08:05:00Z" };

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const sse = (text: string) => new Response(text, { status: 200, headers: { "Content-Type": "text/event-stream" } });

function gatewayReturning(...responses: (Response | Error)[]) {
  const fetchFn = vi.fn<typeof fetch>();
  for (const r of responses) {
    if (r instanceof Error) fetchFn.mockRejectedValueOnce(r);
    else fetchFn.mockResolvedValueOnce(r);
  }
  return { backend: gatewayWatchlist({ gatewayUrl: "http://gw:8080", fetchFn }), fetchFn };
}

const requestOf = (fetchFn: ReturnType<typeof vi.fn<typeof fetch>>, call = 0) => {
  const [url, init] = fetchFn.mock.calls[call];
  return {
    url: String(url),
    method: init?.method,
    headers: (init?.headers ?? {}) as Record<string, string>,
    body: init?.body ? JSON.parse(String(init.body)) : undefined,
  };
};

async function kind(promise: Promise<unknown>) {
  try {
    await promise;
    return "no error";
  } catch (err) {
    return err instanceof HoldError ? err.kind : String(err);
  }
}

async function collect<T>(gen: AsyncGenerator<T>, max = Infinity): Promise<T[]> {
  const out: T[] = [];
  for await (const item of gen) {
    out.push(item);
    if (out.length >= max) break;
  }
  return out;
}

describe("watchlistBackend", () => {
  it("uses the demo without a gateway and the gateway with one", () => {
    expect(watchlistBackend("").source).toBe("demo");
    expect(watchlistBackend("http://gw:8080").source).toBe("gateway");
  });
});

describe("gateway watchlist", () => {
  it("subscribes, lists and unsubscribes with the bearer token", async () => {
    const { backend, fetchFn } = gatewayReturning(json(201, sub), json(200, { subscriptions: [sub] }), json(204));

    expect(await backend.subscribe(session, input)).toEqual(sub);
    expect(await backend.list(session)).toEqual([sub]);
    await backend.unsubscribe(session, "s/1");

    expect(requestOf(fetchFn, 0)).toMatchObject({
      url: "http://gw:8080/api/v1/watchlist",
      method: "POST",
      headers: { Authorization: "Bearer tok-1", "Content-Type": "application/json" },
      body: input,
    });
    expect(requestOf(fetchFn, 1)).toMatchObject({ url: "http://gw:8080/api/v1/watchlist", method: "GET" });
    expect(requestOf(fetchFn, 2)).toMatchObject({ url: "http://gw:8080/api/v1/watchlist/s%2F1", method: "DELETE" });
  });

  it("reads an empty list and rejects a subscription without an ID", async () => {
    const { backend } = gatewayReturning(json(200, {}), json(201, { productId: "p-1" }));
    expect(await backend.list(session)).toEqual([]);
    expect(await kind(backend.subscribe(session, input))).toBe("server");
  });

  it("explains gateway errors, network failures and timeouts", async () => {
    const timeout = Object.assign(new Error("slow"), { name: "TimeoutError" });
    const { backend } = gatewayReturning(json(401), json(404), json(400), new TypeError("fetch failed"), timeout);
    expect(await kind(backend.list(session))).toBe("unauthorized");
    expect(await kind(backend.unsubscribe(session, "s-1"))).toBe("not_found");
    expect(await kind(backend.subscribe(session, input))).toBe("bad_request");
    expect(await kind(backend.list(session))).toBe("unreachable");
    expect(await kind(backend.list(session))).toBe("timeout");
  });

  it("reads alerts from the gateway's event stream, skipping heartbeats", async () => {
    const body = `: connected\n\nid: s-1\nevent: alert\ndata: ${JSON.stringify(alert)}\n\n: ping\n\n`;
    const { backend, fetchFn } = gatewayReturning(sse(body));
    const signal = new AbortController().signal;

    expect(await collect(backend.alerts(session, signal))).toEqual([alert]);
    const [url, init] = fetchFn.mock.calls[0];
    expect(String(url)).toBe("http://gw:8080/api/v1/watchlist/alerts");
    expect((init?.headers as Record<string, string>).Authorization).toBe("Bearer tok-1");
    // The stream lives as long as the caller wants, not the short request timeout.
    expect(init?.signal).toBe(signal);
  });

  it("fails on the gateway's error event and when the stream cannot open", async () => {
    const { backend } = gatewayReturning(sse('event: error\ndata: {"error":"service unavailable"}\n\n'), json(401));
    const signal = new AbortController().signal;
    expect(await kind(collect(backend.alerts(session, signal)))).toBe("server");
    expect(await kind(collect(backend.alerts(session, signal)))).toBe("unauthorized");
  });
});

describe("demo watchlist", () => {
  const product = MOCK_PRODUCTS[0].productId;

  async function signedIn() {
    return demoBackend().login(DEMO_ACCOUNT.email, DEMO_ACCOUNT.password);
  }

  it("brings a watched medicine back after the demo delay, then alerts once", async () => {
    let now = Date.parse("2026-11-11T08:00:00Z");
    const demo = new DemoWatchlist(() => now, 1);
    const me = await signedIn();

    const watched = await demo.subscribe(me, { productId: product, location: thimphu, radiusM: 5000 });
    expect(watched.notifiedAt).toBeUndefined();
    // Asking again while waiting gives the same subscription.
    expect((await demo.subscribe(me, { productId: product, location: thimphu, radiusM: 5000 })).subscriptionId).toBe(watched.subscriptionId);
    expect(await demo.list(me)).toHaveLength(1);

    now += DEMO_RESTOCK_MS;
    const [listed] = await demo.list(me);
    expect(listed.notifiedAt).toBe("2026-11-11T08:00:10.000Z");
    expect(listed).not.toHaveProperty("userId");

    const ac = new AbortController();
    const [first] = await collect(demo.alerts(me, ac.signal), 1);
    expect(first).toMatchObject({ subscriptionId: watched.subscriptionId, productId: product });
    expect(first.quantityAvailable).toBeGreaterThan(0);

    // Already delivered: a new stream gets nothing and ends when aborted.
    const again = demo.alerts(me, ac.signal);
    setTimeout(() => ac.abort(), 5);
    expect(await collect(again)).toEqual([]);
  });

  it("keeps each patient's watchlist to themselves", async () => {
    const demo = new DemoWatchlist();
    const me = await signedIn();
    const watched = await demo.subscribe(me, { productId: product, location: thimphu, radiusM: 5000 });

    await demoBackend().register("other@example.bt", "other-password");
    const other = await demoBackend().login("other@example.bt", "other-password");
    expect(await demo.list(other)).toEqual([]);
    expect(await kind(demo.unsubscribe(other, watched.subscriptionId))).toBe("not_found");

    await demo.unsubscribe(me, watched.subscriptionId);
    expect(await demo.list(me)).toEqual([]);
  });

  it("needs a signed-in patient and a medicine", async () => {
    const demo = new DemoWatchlist();
    expect(await kind(demo.list(session))).toBe("unauthorized");
    expect(await kind(demo.subscribe(await signedIn(), { ...input, productId: "" }))).toBe("bad_request");
  });
});
