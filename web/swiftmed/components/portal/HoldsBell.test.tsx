import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountMenu } from "../AccountMenu";
import { expectNoSeriousViolations } from "../test-utils";

import { HoldsBell, type WaitingHold } from "./HoldsBell";

vi.mock("next/navigation", () => ({ usePathname: () => "/pharmacy-portal", useRouter: () => ({ refresh: vi.fn() }) }));

afterEach(cleanup);

const holds: WaitingHold[] = [
  { holdId: "h1", medicine: "Demo Paracetamol 500", quantity: 2, timeLeft: "1 h 20 min left" },
  { holdId: "h2", medicine: "Demo Amoxicillin 250", quantity: 1, timeLeft: "12 min left" },
];

describe("HoldsBell", () => {
  it("counts the holds waiting, without opening", () => {
    render(<HoldsBell holds={holds} />);
    expect(screen.getByRole("button", { name: "Holds waiting, 2" }).getAttribute("aria-expanded")).toBe("false");
  });

  it("lists each hold by medicine and links to it on the portal's Holds page", () => {
    render(<HoldsBell holds={holds} />);
    fireEvent.click(screen.getByRole("button", { name: /Holds waiting/ }));
    const panel = screen.getByRole("region", { name: "Holds waiting" });
    expect(panel.textContent).toContain("2 units · 1 h 20 min left");
    expect(panel.textContent).toContain("1 unit · 12 min left");
    expect(screen.getByRole("link", { name: /Demo Amoxicillin 250/ }).getAttribute("href")).toBe("/pharmacy-portal/holds#hold-h2");
    expect(screen.getByRole("link", { name: "Collect holds" }).getAttribute("href")).toBe("/pharmacy-portal/holds");
  });

  it("says when no one is waiting, and when loading failed", () => {
    const { rerender } = render(<HoldsBell holds={[]} />);
    fireEvent.click(screen.getByRole("button", { name: "Holds waiting" }));
    expect(screen.getByRole("region", { name: "Holds waiting" }).textContent).toContain("No one is waiting");
    rerender(<HoldsBell holds={[]} failed />);
    expect(screen.getByRole("region", { name: "Holds waiting" }).textContent).toContain("could not be loaded");
  });

  it("closes on Escape and gives focus back to the bell", () => {
    render(<HoldsBell holds={holds} />);
    const bell = screen.getByRole("button", { name: /Holds waiting/ });
    fireEvent.click(bell);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("region", { name: "Holds waiting" })).toBeNull();
    expect(document.activeElement).toBe(bell);
  });

  it("has no serious accessibility violations, open", async () => {
    const { container } = render(<HoldsBell holds={holds} />);
    fireEvent.click(screen.getByRole("button", { name: /Holds waiting/ }));
    await expectNoSeriousViolations(container);
  });
});

describe("AccountMenu for staff", () => {
  it("shows the pharmacy the staff member works at", () => {
    render(<AccountMenu email="demo-tim-001@demo.swiftmeds.test" initial="D" pharmacy="Demo Pharmacy Thimphu 01" />);
    fireEvent.click(screen.getByRole("button", { name: /Account/ }));
    expect(screen.getByRole("region", { name: "Account" }).textContent).toContain("Pharmacy staff · Demo Pharmacy Thimphu 01");
  });
});
