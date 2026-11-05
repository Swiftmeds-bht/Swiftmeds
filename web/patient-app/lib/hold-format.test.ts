import { describe, expect, it } from "vitest";

import { STATUS_INFO, errorMessage, formatClock, formatRemaining, holdNames, statusInfo } from "./hold-format";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "./mock-data";

describe("statusInfo", () => {
  it("gives every status its own label", () => {
    const labels = Object.values(STATUS_INFO).map((s) => s.label);
    expect(new Set(labels).size).toBe(labels.length);
  });

  it("marks only pending and active as not final", () => {
    const open = Object.entries(STATUS_INFO).filter(([, s]) => !s.final).map(([k]) => k);
    expect(open.sort()).toEqual(["HOLD_STATUS_ACTIVE", "HOLD_STATUS_PENDING"]);
  });

  it("copes with a status it does not know", () => {
    expect(statusInfo("HOLD_STATUS_SOMETHING_NEW")).toMatchObject({ label: "Unknown", final: true });
  });
});

describe("formatRemaining", () => {
  it.each([
    [29 * 60_000 + 41_000, "29:41"],
    [59_000, "0:59"],
    [500, "0:01"],
    [0, "0:00"],
    [-5000, "0:00"],
    [65 * 60_000, "1 h 05 min"],
  ])("%i ms is %s", (ms, want) => {
    expect(formatRemaining(ms)).toBe(want);
  });
});

describe("formatClock", () => {
  it("shows Bhutan time", () => {
    expect(formatClock("2026-11-05T08:32:00Z")).toBe("14:32");
  });

  it("is empty without a valid time", () => {
    expect(formatClock(undefined)).toBe("");
    expect(formatClock("soon")).toBe("");
  });
});

describe("errorMessage", () => {
  it("has a different sentence for each kind", () => {
    const kinds = ["invalid_credentials", "email_taken", "unauthorized", "not_found", "not_active", "bad_request", "not_available", "unreachable", "timeout", "server"] as const;
    expect(new Set(kinds.map((k) => errorMessage(k))).size).toBe(kinds.length);
  });

  it("passes on the backend's explanation of bad input only", () => {
    expect(errorMessage("bad_request", "Choose 1 to 5 units")).toBe("Choose 1 to 5 units");
    expect(errorMessage("server", "stack trace")).not.toContain("stack trace");
  });
});

describe("holdNames", () => {
  it("names known medicines and pharmacies", () => {
    const [product, pharmacy] = [MOCK_PRODUCTS[0], MOCK_PHARMACIES[0]];
    expect(holdNames({ productId: product.productId, pharmacyId: pharmacy.pharmacyId })).toEqual({
      medicine: `${product.brandName} ${product.strength}`,
      pharmacy: pharmacy.name,
      address: pharmacy.address,
    });
  });

  it("falls back to plain words", () => {
    expect(holdNames({ productId: "x", pharmacyId: "y" })).toEqual({ medicine: "Medicine", pharmacy: "Pharmacy", address: undefined });
  });
});
