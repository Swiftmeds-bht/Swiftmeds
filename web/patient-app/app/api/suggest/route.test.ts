import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

beforeEach(() => vi.stubEnv("GATEWAY_URL", "")); // demo mode: no network
afterEach(() => vi.unstubAllEnvs());

const ask = (prefix?: string) =>
  GET(new Request(`http://localhost/api/suggest${prefix === undefined ? "" : `?prefix=${encodeURIComponent(prefix)}`}`));

describe("GET /api/suggest", () => {
  it("returns suggestions for a prefix and says they must not be cached", async () => {
    const res = await ask("paracet");
    const body = (await res.json()) as { suggestions: { productId: string; displayName: string }[] };

    expect(res.status).toBe(200);
    expect(res.headers.get("Cache-Control")).toBe("no-store");
    expect(body.suggestions.map((s) => s.displayName)).toContain("Demo Paracetamol 500 (Paracetamol)");
  });

  it.each([["p"], [""], [undefined], ["x".repeat(101)]])("returns an empty list for prefix %j", async (prefix) => {
    const res = await ask(prefix);
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ suggestions: [] });
  });
});
