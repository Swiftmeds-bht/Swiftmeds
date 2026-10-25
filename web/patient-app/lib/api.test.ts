import { describe, expect, it, vi } from "vitest";

import { distanceM, getPharmacy, mockSearch, searchMedicines, searchUrl } from "./api";
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

describe("searchMedicines", () => {
  it("uses demo data when no gateway is configured", async () => {
    const fetchFn = vi.fn();
    const res = await searchMedicines(params("paracetamol"), { gatewayUrl: "", fetchFn });
    expect(res.source).toBe("demo");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("uses gateway data when the gateway answers", async () => {
    const body = { searchId: "s-1", results: [{ productId: "p", brandName: "B", genericName: "G", strength: "1mg", offers: [] }] };
    const fetchFn = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
    const res = await searchMedicines(params("x"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.source).toBe("gateway");
    expect(res.data.searchId).toBe("s-1");
    expect(res.data.results).toHaveLength(1);
    expect(res.data.substitutes).toEqual([]); // missing arrays are filled in
  });

  it("falls back to demo data while the gateway route does not exist (404)", async () => {
    const fetchFn = vi.fn().mockResolvedValue(new Response("not found", { status: 404 }));
    const res = await searchMedicines(params("paracetamol"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.source).toBe("demo");
    expect(res.data.results.length).toBeGreaterThan(0);
  });

  it("falls back to demo data when the gateway is unreachable", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
    const res = await searchMedicines(params("paracetamol"), { gatewayUrl: "http://gw", fetchFn });
    expect(res.source).toBe("demo");
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
