import { describe, expect, it, vi } from "vitest";

import { timeLeft } from "./format";
import { fetchMedicineName, fetchPharmacyName, medicineName, UNKNOWN_MEDICINE } from "./names";

describe("medicineName", () => {
  it("shows the brand, then strength, form and the generic name", () => {
    expect(medicineName({ brandName: "Demo Paracetamol 500", genericName: "Paracetamol", strength: "500mg", dosageForm: "DOSAGE_FORM_TABLET" })).toEqual({
      name: "Demo Paracetamol 500",
      detail: "500mg tablet · Paracetamol",
      known: true,
    });
  });

  it("does not repeat a generic name that equals the brand", () => {
    expect(medicineName({ brandName: "Amoxicillin", genericName: "amoxicillin", strength: "250mg" }).detail).toBe("250mg");
  });

  it("falls back to the generic name, and to Unknown medicine", () => {
    expect(medicineName({ genericName: "Paracetamol" }).name).toBe("Paracetamol");
    expect(medicineName(null)).toEqual(UNKNOWN_MEDICINE);
    expect(medicineName({ brandName: "  " })).toEqual(UNKNOWN_MEDICINE);
  });
});

describe("lookups", () => {
  it("asks the public product route and names the medicine", async () => {
    const fetchFn = vi.fn(async () => Response.json({ brandName: "Demo Paracetamol 500", strength: "500mg" }));
    const name = await fetchMedicineName("p 1", fetchFn);
    expect(String((fetchFn.mock.calls[0] as unknown as [URL])[0])).toMatch(/\/api\/v1\/products\/p%201$/);
    expect(name).toMatchObject({ name: "Demo Paracetamol 500", known: true });
  });

  it("never breaks the page: unknown IDs, errors and timeouts give the fallback", async () => {
    expect(await fetchMedicineName("x", vi.fn(async () => new Response(null, { status: 404 })))).toEqual(UNKNOWN_MEDICINE);
    expect(await fetchMedicineName("x", vi.fn(async () => new Response(null, { status: 503 })))).toEqual(UNKNOWN_MEDICINE);
    expect(
      await fetchMedicineName(
        "x",
        vi.fn(async () => {
          throw new TypeError("fetch failed");
        }),
      ),
    ).toEqual(UNKNOWN_MEDICINE);
    expect(await fetchMedicineName("", vi.fn())).toEqual(UNKNOWN_MEDICINE);
  });

  it("names a pharmacy, or gives nothing when it cannot", async () => {
    expect(await fetchPharmacyName("ph1", vi.fn(async () => Response.json({ name: "Demo Pharmacy Thimphu 01" })))).toBe("Demo Pharmacy Thimphu 01");
    expect(await fetchPharmacyName("ph1", vi.fn(async () => new Response(null, { status: 404 })))).toBeUndefined();
  });
});

describe("timeLeft", () => {
  const now = Date.parse("2026-11-19T12:00:00Z");
  it.each([
    ["2026-11-19T12:12:00Z", "12 min left"],
    ["2026-11-19T13:20:00Z", "1 h 20 min left"],
    ["2026-11-19T14:00:00Z", "2 h left"],
    ["2026-11-19T11:59:00Z", "Expired"],
    [undefined, ""],
  ])("%s reads as %s", (expiresAt, text) => {
    expect(timeLeft(expiresAt, now)).toBe(text);
  });
});
