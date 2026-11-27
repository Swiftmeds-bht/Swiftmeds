import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import PharmacyPage from "@/app/pharmacy/[id]/page";
import * as api from "@/lib/api";

import { expectNoSeriousViolations } from "./test-utils";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  lookupPharmacy: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
}));

const lookup = vi.mocked(api.lookupPharmacy);
const show = async (id = "p-1") => render(await PharmacyPage({ params: Promise.resolve({ id }) }));

beforeEach(() => {
  lookup.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("pharmacy page", () => {
  it("shows the live pharmacy in gateway mode, without the demo label", async () => {
    lookup.mockResolvedValue({
      status: "found",
      source: "gateway",
      pharmacy: { pharmacyId: "p-1", name: "Druk Pharmacy", address: "Norzin Lam", dzongkhag: "Thimphu", phone: "+975 2 322 000", location: { latitude: 27.47, longitude: 89.64 } },
    });
    const { container } = await show();
    expect(screen.getByRole("heading", { level: 1, name: "Druk Pharmacy" })).toBeTruthy();
    expect(screen.getByText("Norzin Lam")).toBeTruthy();
    expect(screen.getByRole("link", { name: "+975 2 322 000" }).getAttribute("href")).toBe("tel:+9752322000");
    // Directions first (SCRUM-158); OpenStreetMap as a plain link.
    expect(screen.getByRole("link", { name: /Get directions/ }).getAttribute("href")).toMatch(/^https:\/\/www\.google\.com\/maps\/dir\/\?api=1&destination=/);
    expect(screen.getByRole("link", { name: /OpenStreetMap/ })).toBeTruthy();
    expect(screen.queryByText(/demo pharmacy/i)).toBeNull();
    await expectNoSeriousViolations(container);
  });

  it("labels demo data in demo mode", async () => {
    const demo = api.getPharmacy("40000000-0000-4000-8000-000000000011")!;
    lookup.mockResolvedValue({ status: "found", source: "demo", pharmacy: demo });
    await show(demo.pharmacyId);
    expect(screen.getByRole("heading", { level: 1, name: demo.name })).toBeTruthy();
    expect(screen.getByText("Showing a demo pharmacy, not live data.")).toBeTruthy();
  });

  it("works without a location or address", async () => {
    lookup.mockResolvedValue({ status: "found", source: "gateway", pharmacy: { pharmacyId: "p-1", name: "Druk Pharmacy" } });
    await show();
    expect(screen.queryByRole("link", { name: /Get directions/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /OpenStreetMap/ })).toBeNull();
    expect(screen.queryByText("Address")).toBeNull();
  });

  it("says details are not available instead of showing demo data when the gateway cannot answer", async () => {
    lookup.mockResolvedValue({ status: "unavailable", kind: "not_available" });
    const { container } = await show();
    expect(screen.getByRole("alert").textContent).toContain("Pharmacy details are not available right now");
    expect(screen.queryByText(/Demo Pharmacy/)).toBeNull();
    await expectNoSeriousViolations(container);
  });

  it("is not found for an unknown pharmacy", async () => {
    lookup.mockResolvedValue({ status: "not_found" });
    await expect(show()).rejects.toThrow("NEXT_NOT_FOUND");
  });
});
