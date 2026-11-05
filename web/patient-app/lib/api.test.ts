import { describe, expect, it, vi } from "vitest";

import {
  SearchError,
  autocompleteUrl,
  distanceM,
  fetchSuggestions,
  getPharmacy,
  mockSearch,
  mockSuggest,
  searchMedicines,
  searchUrl,
} from "./api";
import type { SearchParams } from "./types";

const thimphu = { latitude: 27.4728, longitude: 89.639 };
const params = (query: string, radiusM = 5000): SearchParams => ({ query, location: thimphu, radiusM });

describe("searchUrl", () => {
  it("builds the gateway search URL with encoded parameters", () => {
    const url = new URL(searchUrl("http://gateway:8080", params("para cetamol", 2000)));
    expect(url.pathname).toBe("/api/v1/search");
    expect(url.searchParams.get("q")).toBe("para cetamol");
    expect(url.searchParams.get("lat")).toBe("27.4728");
    expect(url.searchParams.get("lng")).toBe("89.639");
    expect(url.searchParams.get("radius_m")).toBe("2000");
  });
});

const json = (body: unknown, init: ResponseInit = { status: 200 }) => new Response(JSON.stringify(body), init);
const kindOf = async (promise: Promise<unknown>) => {
  try {
    await promise;
  } catch (error) {
    if (error instanceof SearchError) return error.kind;
    throw error;
  }
  return "no error";
};

describe("searchMedicines", () => {
  it("uses clearly labelled demo data when no gateway is configured", async () => {
    const fetchFn = vi.fn();
    const res = await searchMedicines(params("paracetamol"), { gatewayUrl: "", fetchFn });
    expect(res.source).toBe("demo");
    expect(res.degraded).toEqual([]);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("uses gateway data when the gateway answers", async () => {
    const body = { searchId: "s-1", results: [{ productId: "p", brandName: "B", genericName: "G", strength: "1mg", offers: [] }] };
    const fetchFn = vi.fn().mockResolvedValue(json(body));
    const res = await searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.source).toBe("gateway");
    expect(res.data.searchId).toBe("s-1");
    expect(res.data.results).toHaveLength(1);
    expect(res.data.substitutes).toEqual([]); // missing arrays are filled in
    expect(fetchFn).toHaveBeenCalledOnce();
    expect(String(fetchFn.mock.calls[0][0])).toContain("/api/v1/search?q=x");
  });

  it("copes with an empty JSON answer", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(null));
    const res = await searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.data).toEqual({ searchId: "", results: [], substitutes: [] });
  });

  it("reports the optional services that did not answer (x-search-degraded)", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json({}, { status: 200, headers: { "x-search-degraded": "freshness, ranking" } }));
    const res = await searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.degraded).toEqual(["freshness", "ranking"]);
  });

  it.each([
    [400, "bad_request"],
    [404, "not_available"],
    [501, "not_available"],
    [408, "timeout"],
    [504, "timeout"],
    [500, "server"],
    [503, "server"],
    [429, "server"],
  ])("turns HTTP %i into a %s error", async (status, kind) => {
    const fetchFn = vi.fn().mockResolvedValue(new Response("x", { status }));
    expect(await kindOf(searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn }))).toBe(kind);
  });

  it("turns a network failure into an unreachable error", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
    expect(await kindOf(searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn }))).toBe("unreachable");
  });

  it.each(["TimeoutError", "AbortError"])("turns a %s into a timeout error", async (name) => {
    const fetchFn = vi.fn().mockRejectedValue(new DOMException("too slow", name));
    expect(await kindOf(searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn }))).toBe("timeout");
  });

  it("turns an answer that is not JSON into a server error", async () => {
    const fetchFn = vi.fn().mockResolvedValue(new Response("<html>oops</html>", { status: 200 }));
    expect(await kindOf(searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn }))).toBe("server");
  });

  it("never falls back to demo data when a configured gateway fails", async () => {
    const fetchFn = vi.fn().mockResolvedValue(new Response("not found", { status: 404 }));
    await expect(searchMedicines(params("paracetamol"), { gatewayUrl: "http://gw", fetchFn })).rejects.toBeInstanceOf(SearchError);
  });
});

describe("autocomplete", () => {
  it("builds the gateway autocomplete URL", () => {
    const url = new URL(autocompleteUrl("http://gateway:8080", "para cet", 5));
    expect(url.pathname).toBe("/api/v1/search/autocomplete");
    expect(url.searchParams.get("prefix")).toBe("para cet");
    expect(url.searchParams.get("limit")).toBe("5");
  });

  it("returns the gateway's suggestions and drops malformed ones", async () => {
    const body = { suggestions: [{ productId: "p1", displayName: "Napa (Paracetamol)" }, { productId: 7 }, null, "x"] };
    const fetchFn = vi.fn().mockResolvedValue(json(body));
    const got = await fetchSuggestions("nap", { gatewayUrl: "http://gw", fetchFn });
    expect(got).toEqual([{ productId: "p1", displayName: "Napa (Paracetamol)" }]);
  });

  it.each([
    ["the gateway fails", () => new Response("x", { status: 500 })],
    ["the route does not exist yet", () => new Response("x", { status: 404 })],
    ["the answer is not JSON", () => new Response("<html>", { status: 200 })],
    ["the answer has no list", () => json({ suggestions: "none" })],
    ["the answer is empty", () => json(null)],
  ])("gives an empty list when %s", async (_name, respond) => {
    const fetchFn = vi.fn().mockResolvedValue(respond());
    expect(await fetchSuggestions("nap", { gatewayUrl: "http://gw", fetchFn })).toEqual([]);
  });

  it("gives an empty list when the gateway is unreachable", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
    expect(await fetchSuggestions("nap", { gatewayUrl: "http://gw", fetchFn })).toEqual([]);
  });

  it.each([["p"], ["  "], [""], ["x".repeat(101)]])("does not call the gateway for %j", async (prefix) => {
    const fetchFn = vi.fn();
    expect(await fetchSuggestions(prefix, { gatewayUrl: "http://gw", fetchFn })).toEqual([]);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("uses demo suggestions when no gateway is configured", async () => {
    const got = await fetchSuggestions("paracet", { gatewayUrl: "" });
    expect(got.map((s) => s.displayName)).toContain("Demo Paracetamol 500 (Paracetamol)");
  });
});

describe("mockSuggest", () => {
  it("matches brand or generic names, ignores case and respects the limit", () => {
    expect(mockSuggest("IBUPROFEN").map((s) => s.displayName)).toEqual(["Demo Ibuprofen 400 (Ibuprofen)"]);
    expect(mockSuggest("demo", 3)).toHaveLength(3);
    expect(mockSuggest("nonexistium")).toEqual([]);
    expect(mockSuggest("  ")).toEqual([]);
  });
});

describe("mockSearch (demo data on the seed)", () => {
  it("finds by generic name, case-insensitive, with offers nearest first and in stock only", () => {
    const { results } = mockSearch(params("PARACETAMOL", 5000));
    const tablets = results.find((r) => r.brandName === "Demo Paracetamol 500");
    expect(tablets).toBeDefined();
    const distances = tablets!.offers.map((o) => o.distanceM);
    expect(distances).toEqual([...distances].sort((a, b) => a - b));
    expect(tablets!.offers.every((o) => o.quantityAvailable > 0)).toBe(true);
    // Thimphu 01 has 0 units, so it must not be listed.
    expect(tablets!.offers.map((o) => o.pharmacyName)).not.toContain("Demo Pharmacy Thimphu 01");
  });

  it("respects the radius: Paro stock appears only within 50 km", () => {
    const near = mockSearch(params("Demo Paracetamol 500", 5000)).results[0];
    const wide = mockSearch(params("Demo Paracetamol 500", 50000)).results[0];
    expect(near.offers.some((o) => o.pharmacyName.includes("Paro"))).toBe(false);
    expect(wide.offers.some((o) => o.pharmacyName.includes("Paro"))).toBe(true);
  });

  it("returns nothing for an unknown medicine or an empty query", () => {
    expect(mockSearch(params("nonexistium")).results).toEqual([]);
    expect(mockSearch(params("   ")).results).toEqual([]);
  });

  it("suggests same-molecule, same-strength products only when nothing matched is in stock", () => {
    // Amlodipine is in stock in Thimphu: no substitutes needed.
    expect(mockSearch(params("amlodipine")).substitutes).toEqual([]);
  });
});

describe("distanceM", () => {
  it("is about 73.4 km from Thimphu to Phuentsholing", () => {
    const d = distanceM(thimphu, { latitude: 26.8516, longitude: 89.3884 });
    expect(Math.abs(d - 73_390) / 73_390).toBeLessThan(0.01);
  });
});

describe("getPharmacy", () => {
  it("finds seed pharmacies and returns undefined for unknown IDs", () => {
    expect(getPharmacy("40000000-0000-4000-8000-000000000011")?.name).toBe("Demo Pharmacy Phuentsholing 01");
    expect(getPharmacy("nope")).toBeUndefined();
  });
});
