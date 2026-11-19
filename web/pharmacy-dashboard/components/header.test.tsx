import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountMenu } from "./AccountMenu";
import { HoldsBell, type WaitingHold } from "./HoldsBell";
import { MainNav } from "./MainNav";
import { expectNoSeriousViolations } from "./test-utils";

const nav = vi.hoisted(() => ({ path: "/" }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.path, redirect: vi.fn() }));
vi.mock("@/app/actions", () => ({ logoutAction: vi.fn() }));

afterEach(cleanup);

describe("MainNav", () => {
  it.each([
    ["/", "Overview"],
    ["/holds", "Holds"],
    ["/stock", "Stock"],
    ["/upload", "Upload CSV"],
  ])("marks %s as %s", (path, current) => {
    nav.path = path;
    render(<MainNav />);
    const marked = screen.getAllByRole("link").filter((l) => l.getAttribute("aria-current") === "page");
    expect(marked.map((l) => l.textContent)).toEqual([current]);
  });
});

const holds: WaitingHold[] = [
  { holdId: "h1", medicine: "Demo Paracetamol 500", quantity: 2, timeLeft: "1 h 20 min left" },
  { holdId: "h2", medicine: "Demo Amoxicillin 250", quantity: 1, timeLeft: "12 min left" },
];

describe("HoldsBell", () => {
  it("counts the holds waiting, without opening", () => {
    render(<HoldsBell holds={holds} />);
    expect(screen.getByRole("button", { name: "Holds waiting, 2" }).getAttribute("aria-expanded")).toBe("false");
  });

  it("lists each hold by medicine and links to it on the Holds page", () => {
    render(<HoldsBell holds={holds} />);
    fireEvent.click(screen.getByRole("button", { name: /Holds waiting/ }));
    const panel = screen.getByRole("region", { name: "Holds waiting" });
    expect(panel.textContent).toContain("Demo Paracetamol 500");
    expect(panel.textContent).toContain("2 units · 1 h 20 min left");
    expect(panel.textContent).toContain("1 unit · 12 min left");
    expect(screen.getByRole("link", { name: /Demo Amoxicillin 250/ }).getAttribute("href")).toBe("/holds#hold-h2");
    expect(screen.getByRole("link", { name: "Collect holds" }).getAttribute("href")).toBe("/holds");
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

describe("AccountMenu", () => {
  it("shows the pharmacy and the staff email, and a small Sign out", () => {
    render(<AccountMenu pharmacy="Demo Pharmacy Thimphu 01" email="demo-tim-001@demo.swiftmeds.test" />);
    const button = screen.getByRole("button", { name: "Account: demo-tim-001@demo.swiftmeds.test" });
    expect(button.textContent).toContain("D");
    fireEvent.click(button);
    const panel = screen.getByRole("region", { name: "Account" });
    expect(panel.textContent).toContain("Demo Pharmacy Thimphu 01");
    expect(panel.textContent).toContain("demo-tim-001@demo.swiftmeds.test");
    expect(screen.getByRole("button", { name: "Sign out" })).toBeTruthy();
  });

  it("works for an older session without an email", () => {
    render(<AccountMenu pharmacy="Demo Pharmacy Paro 02" />);
    fireEvent.click(screen.getByRole("button", { name: "Account: Demo Pharmacy Paro 02" }));
    expect(screen.getByRole("region", { name: "Account" }).textContent).toContain("Pharmacy staff");
  });

  it("closes the bell when it opens: one panel at a time", () => {
    render(
      <>
        <HoldsBell holds={holds} />
        <AccountMenu pharmacy="Demo Pharmacy Thimphu 01" />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: /Holds waiting/ }));
    fireEvent.click(screen.getByRole("button", { name: /Account/ }));
    expect(screen.queryByRole("region", { name: "Holds waiting" })).toBeNull();
    expect(screen.getByRole("region", { name: "Account" })).toBeTruthy();
  });

  it("has no serious accessibility violations, open", async () => {
    const { container } = render(<AccountMenu pharmacy="Demo Pharmacy Thimphu 01" email="demo-tim-001@demo.swiftmeds.test" />);
    fireEvent.click(screen.getByRole("button", { name: /Account/ }));
    await expectNoSeriousViolations(container);
  });
});
