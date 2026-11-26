import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "@/lib/api";

import { GettingThere } from "./GettingThere";
import { expectNoSeriousViolations } from "./test-utils";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  lookupPharmacy: vi.fn(),
}));

const lookup = vi.mocked(api.lookupPharmacy);
afterEach(() => {
  cleanup();
  lookup.mockReset();
});

describe("GettingThere (SCRUM-158)", () => {
  it("names the pharmacy and links to directions to it", async () => {
    lookup.mockResolvedValue({
      status: "found",
      source: "gateway",
      pharmacy: { pharmacyId: "ph-1", name: "Druk Pharmacy", address: "Norzin Lam", location: { latitude: 27.4728, longitude: 89.639 } },
    });
    const { container } = render((await GettingThere({ pharmacyId: "ph-1" }))!);
    expect(screen.getByRole("heading", { name: "Getting there" })).toBeTruthy();
    expect(screen.getByText("Druk Pharmacy")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Get directions/ }).getAttribute("href")).toBe(
      "https://www.google.com/maps/dir/?api=1&destination=27.4728,89.639&travelmode=driving",
    );
    await expectNoSeriousViolations(container);
  });

  it("shows nothing when the pharmacy cannot be looked up, or has no address or location", async () => {
    lookup.mockResolvedValue({ status: "unavailable", kind: "timeout" });
    expect(await GettingThere({ pharmacyId: "ph-1" })).toBeNull();
    lookup.mockResolvedValue({ status: "found", source: "gateway", pharmacy: { pharmacyId: "ph-1", name: "Druk Pharmacy" } });
    expect(await GettingThere({ pharmacyId: "ph-1" })).toBeNull();
    expect(await GettingThere({})).toBeNull();
  });
});
