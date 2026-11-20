import { describe, expect, it, vi } from "vitest";

import { holdNames } from "./hold-format";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "./mock-data";
import { EMPTY_BOOK, nameBook } from "./names";

const gw = { gatewayUrl: "http://gw.test" };

function fakeGateway(products: Record<string, object>, pharmacies: Record<string, object>) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(input instanceof Request ? input.url : String(input)).pathname;
    const id = decodeURIComponent(path.split("/").pop() ?? "");
    const body = path.startsWith("/api/v1/products/") ? products[id] : pharmacies[id];
    return body ? Response.json(body) : new Response(null, { status: 404 });
  });
}

describe("nameBook", () => {
  it("looks up each medicine and pharmacy once, through the public routes", async () => {
    const fetchFn = fakeGateway({ p9: { brandName: "Real Brand", strength: "250mg" } }, { ph9: { name: "Real Pharmacy", address: "Paro town" } });
    const book = await nameBook(
      [
        { productId: "p9", pharmacyId: "ph9" },
        { productId: "p9", pharmacyId: "ph9" },
      ],
      { ...gw, fetchFn },
    );
    expect(fetchFn).toHaveBeenCalledTimes(2);
    expect(holdNames({ productId: "p9", pharmacyId: "ph9" }, book)).toEqual({ medicine: "Real Brand 250mg", pharmacy: "Real Pharmacy", address: "Paro town" });
  });

  it("asks nothing in demo mode: the demo data names the hold", async () => {
    const fetchFn = vi.fn();
    expect(await nameBook([{ productId: "p1" }], { gatewayUrl: "", fetchFn })).toBe(EMPTY_BOOK);
    expect(fetchFn).not.toHaveBeenCalled();
    const demo = { productId: MOCK_PRODUCTS[0].productId, pharmacyId: MOCK_PHARMACIES[0].pharmacyId };
    expect(holdNames(demo).medicine).toContain(MOCK_PRODUCTS[0].brandName);
  });

  it("falls back when a lookup fails, and never shows a missing strength as undefined", async () => {
    const fetchFn = fakeGateway({ p8: { brandName: "No Strength" } }, {});
    const book = await nameBook([{ productId: "p8", pharmacyId: "gone" }, { productId: "gone" }], { ...gw, fetchFn });
    expect(holdNames({ productId: "p8", pharmacyId: "gone" }, book)).toEqual({ medicine: "No Strength", pharmacy: "Pharmacy", address: undefined });
    expect(holdNames({ productId: "gone", pharmacyId: "gone" }, book).medicine).toBe("Medicine");
  });

  it("survives a gateway that cannot be reached", async () => {
    const fetchFn = vi.fn(async () => {
      throw new TypeError("fetch failed");
    });
    const book = await nameBook([{ productId: "p1", pharmacyId: "ph1" }], { ...gw, fetchFn });
    expect(book.products.size + book.pharmacies.size).toBe(0);
  });
});
