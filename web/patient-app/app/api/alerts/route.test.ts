import { afterEach, describe, expect, it, vi } from "vitest";

import * as auth from "@/lib/auth";
import { HoldError } from "@/lib/holds";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "@/lib/mock-data";
import { parseSSE, type SSEEvent } from "@/lib/sse";
import type { StockAlert } from "@/lib/types";
import * as watchlist from "@/lib/watchlist";

import { GET } from "./route";

vi.mock("@/lib/auth", () => ({ getSession: vi.fn() }));
vi.mock("@/lib/watchlist", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/watchlist")>()),
  watchlistBackend: vi.fn(),
}));

const session = { userId: "u-1", email: "tashi@example.bt", accessToken: "tok-1" };
const alert: StockAlert = {
  subscriptionId: "s-1",
  productId: MOCK_PRODUCTS[0].productId,
  pharmacyId: MOCK_PHARMACIES[0].pharmacyId,
  quantityAvailable: 4,
  occurredAt: "2026-11-11T08:05:00Z",
};

/** A backend whose alert stream yields `alerts`, then fails with `end` or waits until aborted. */
function backendWith(alerts: StockAlert[], end?: Error) {
  const seen: { signal?: AbortSignal } = {};
  vi.mocked(watchlist.watchlistBackend).mockReturnValue({
    source: "gateway",
    subscribe: vi.fn(),
    list: vi.fn(),
    unsubscribe: vi.fn(),
    async *alerts(_s, signal) {
      seen.signal = signal;
      yield* alerts;
      if (end) throw end;
      await new Promise((resolve) => signal.addEventListener("abort", resolve, { once: true }));
    },
  });
  return seen;
}

async function firstEvents(res: Response, count: number): Promise<SSEEvent[]> {
  const out: SSEEvent[] = [];
  for await (const e of parseSSE(res.body!)) {
    out.push(e);
    if (out.length === count) break;
  }
  return out;
}

afterEach(() => vi.resetAllMocks());

describe("GET /api/alerts", () => {
  it("refuses a visitor who is not signed in", async () => {
    vi.mocked(auth.getSession).mockResolvedValue(undefined);
    const res = await GET(new Request("http://localhost/api/alerts"));
    expect(res.status).toBe(401);
  });

  it("streams alerts with the medicine and pharmacy names", async () => {
    vi.mocked(auth.getSession).mockResolvedValue(session);
    backendWith([alert]);
    const browser = new AbortController();

    const res = await GET(new Request("http://localhost/api/alerts", { signal: browser.signal }));
    expect(res.headers.get("Content-Type")).toBe("text/event-stream");
    expect(res.headers.get("Cache-Control")).toContain("no-cache");

    const [event] = await firstEvents(res, 1);
    expect(event).toMatchObject({ event: "alert", id: "s-1" });
    expect(JSON.parse(event.data)).toEqual({
      ...alert,
      medicine: `${MOCK_PRODUCTS[0].brandName} ${MOCK_PRODUCTS[0].strength}`,
      pharmacy: MOCK_PHARMACIES[0].name,
    });
    browser.abort();
  });

  it("stops asking the gateway when the browser leaves", async () => {
    vi.mocked(auth.getSession).mockResolvedValue(session);
    const seen = backendWith([]);
    const browser = new AbortController();

    const res = await GET(new Request("http://localhost/api/alerts", { signal: browser.signal }));
    const reader = res.body!.getReader();
    await reader.read(); // ": connected"
    expect(seen.signal?.aborted).toBe(false);

    browser.abort();
    expect(seen.signal?.aborted).toBe(true);
    expect((await reader.read()).done).toBe(true);
  });

  it("tells the page why the stream ended", async () => {
    vi.mocked(auth.getSession).mockResolvedValue(session);
    backendWith([], new HoldError("unauthorized", "expired"));
    const res = await GET(new Request("http://localhost/api/alerts"));
    const [event] = await firstEvents(res, 1);
    expect(event).toEqual({ event: "error", data: '{"kind":"unauthorized"}' });
  });
});
