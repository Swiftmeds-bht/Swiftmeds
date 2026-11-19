import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { NotificationView } from "@/lib/notifications";

import { AccountMenu } from "./AccountMenu";
import { MainNav } from "./MainNav";
import { NotificationBell } from "./NotificationBell";
import { ProfileForm } from "./ProfileForm";
import { expectNoSeriousViolations } from "./test-utils";

const nav = vi.hoisted(() => ({ path: "/" }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.path, useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }) }));

afterEach(cleanup);

describe("MainNav", () => {
  it.each([
    ["/", "Search"],
    ["/results", "Search"],
    ["/pharmacy/p1", "Search"],
    ["/holds", "My holds"],
    ["/holds/h1", "My holds"],
    ["/watchlist", "Watchlist"],
  ])("marks %s as %s for a patient", (path, current) => {
    nav.path = path;
    render(<MainNav audience="patient" />);
    const currentLinks = screen.getAllByRole("link").filter((l) => l.getAttribute("aria-current") === "page");
    expect(currentLinks.map((l) => l.textContent)).toEqual([current]);
  });

  it("offers a visitor Search and the way in for pharmacies", () => {
    nav.path = "/";
    render(<MainNav audience="guest" />);
    expect(screen.getAllByRole("link").map((l) => l.textContent)).toEqual(["Search", "For pharmacies"]);
    expect(screen.getByRole("link", { name: "For pharmacies" }).getAttribute("href")).toBe("/pharmacy-portal");
    expect(screen.getByRole("navigation", { name: "Main" })).toBeTruthy();
  });

  it.each([
    ["/pharmacy-portal", "Overview"],
    ["/pharmacy-portal/holds", "Holds"],
    ["/pharmacy-portal/stock", "Stock"],
    ["/pharmacy-portal/upload", "Upload CSV"],
  ])("shows staff the portal's sections, marking %s as %s", (path, current) => {
    nav.path = path;
    render(<MainNav audience="staff" />);
    expect(screen.getAllByRole("link").map((l) => l.textContent)).toEqual(["Overview", "Holds", "Stock", "Upload CSV"]);
    const currentLinks = screen.getAllByRole("link").filter((l) => l.getAttribute("aria-current") === "page");
    expect(currentLinks.map((l) => l.textContent)).toEqual([current]);
  });

  it("does not mark the patient's pharmacy pages as the portal", () => {
    nav.path = "/pharmacy/p1";
    render(<MainNav audience="guest" />);
    expect(screen.getByRole("link", { name: "For pharmacies" }).hasAttribute("aria-current")).toBe(false);
  });

  it("marks nothing on a page outside the sections", () => {
    nav.path = "/profile";
    render(<MainNav audience="patient" />);
    expect(screen.getAllByRole("link").some((l) => l.hasAttribute("aria-current"))).toBe(false);
  });
});

const items: NotificationView[] = [
  { id: "n2", title: "Hold confirmed", body: "Show pickup code 442BCX at the counter.", href: "/holds/h2", createdAt: "2026-11-19T10:00:00Z" },
  { id: "n1", title: "Hold expired", body: "The time to collect ran out, so the units were released.", href: "/holds/h1", createdAt: "2026-11-19T09:00:00Z" },
];

describe("NotificationBell", () => {
  it("says how many are unread, without opening", () => {
    render(<NotificationBell items={items} unread={2} newest="2026-11-19T10:00:00Z" onSeen={vi.fn()} />);
    const bell = screen.getByRole("button", { name: "Notifications, 2 unread" });
    expect(bell.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region", { name: "Notifications" })).toBeNull();
  });

  it("opens the list, marks it seen once and clears the count", async () => {
    const onSeen = vi.fn(async () => {});
    render(<NotificationBell items={items} unread={2} newest="2026-11-19T10:00:00Z" onSeen={onSeen} />);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: /Notifications/ })));

    const panel = screen.getByRole("region", { name: "Notifications" });
    expect(panel.textContent).toContain("442BCX");
    expect(screen.getByRole("link", { name: /Hold confirmed/ }).getAttribute("href")).toBe("/holds/h2");
    expect(onSeen).toHaveBeenCalledWith("2026-11-19T10:00:00Z");
    expect(screen.getByRole("button", { name: "Notifications" }).getAttribute("aria-expanded")).toBe("true");

    // Closing and opening again does not mark it again.
    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    expect(onSeen).toHaveBeenCalledTimes(1);
  });

  it("closes on Escape and gives focus back to the bell", async () => {
    render(<NotificationBell items={items} unread={0} onSeen={vi.fn()} />);
    const bell = screen.getByRole("button", { name: "Notifications" });
    fireEvent.click(bell);
    expect(screen.getByRole("region", { name: "Notifications" })).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("region", { name: "Notifications" })).toBeNull();
    expect(document.activeElement).toBe(bell);
  });

  it("says what to expect when there is nothing yet, and when loading failed", () => {
    const { rerender } = render(<NotificationBell items={[]} unread={0} onSeen={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    expect(screen.getByRole("region", { name: "Notifications" }).textContent).toContain("Hold updates and back-in-stock alerts appear here");
    rerender(<NotificationBell items={[]} unread={0} failed onSeen={vi.fn()} />);
    expect(screen.getByRole("region", { name: "Notifications" }).textContent).toContain("could not be loaded");
  });

  it("has no serious accessibility violations, open", async () => {
    const { container } = render(<NotificationBell items={items} unread={2} newest="2026-11-19T10:00:00Z" onSeen={vi.fn(async () => {})} />);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: /Notifications/ })));
    await expectNoSeriousViolations(container);
  });
});

describe("AccountMenu", () => {
  it("shows who is signed in, the profile link and a small Sign out", () => {
    render(<AccountMenu email="patient@demo.swiftmeds.test" name="Karma" initial="K" />);
    const button = screen.getByRole("button", { name: "Account: Karma" });
    expect(button.textContent).toContain("K");
    fireEvent.click(button);
    const panel = screen.getByRole("region", { name: "Account" });
    expect(panel.textContent).toContain("patient@demo.swiftmeds.test");
    expect(screen.getByRole("link", { name: "Your profile" }).getAttribute("href")).toBe("/profile");
    expect(screen.getByRole("button", { name: "Sign out" })).toBeTruthy();
  });

  it("invites a patient without a name to add one", () => {
    render(<AccountMenu email="patient@demo.swiftmeds.test" initial="P" />);
    fireEvent.click(screen.getByRole("button", { name: "Account: patient@demo.swiftmeds.test" }));
    expect(screen.getByRole("link", { name: "Add your name and phone" })).toBeTruthy();
  });

  it("closes the other panel: only one is open at a time", () => {
    render(
      <>
        <NotificationBell items={items} unread={0} onSeen={vi.fn()} />
        <AccountMenu email="patient@demo.swiftmeds.test" initial="P" />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
    fireEvent.click(screen.getByRole("button", { name: /Account/ }));
    expect(screen.queryByRole("region", { name: "Notifications" })).toBeNull();
    expect(screen.getByRole("region", { name: "Account" })).toBeTruthy();
  });

  it("has no serious accessibility violations, open", async () => {
    const { container } = render(<AccountMenu email="patient@demo.swiftmeds.test" name="Karma" initial="K" />);
    fireEvent.click(screen.getByRole("button", { name: "Account: Karma" }));
    await expectNoSeriousViolations(container);
  });
});

describe("ProfileForm", () => {
  it("labels the fields and shows the saved values", async () => {
    const { container } = render(<ProfileForm displayName="Karma" phone="17123456" action={vi.fn()} />);
    expect(screen.getByLabelText("Name")).toHaveProperty("value", "Karma");
    expect(screen.getByLabelText("Phone")).toHaveProperty("value", "17123456");
    await expectNoSeriousViolations(container);
  });

  it("shows what to fix, keeping what was typed", async () => {
    const action = vi.fn(async () => ({ error: "Enter a phone number with 8 to 15 digits.", displayName: "Karma", phone: "17" }));
    render(<ProfileForm displayName="" phone="" action={action} />);
    await act(async () => fireEvent.submit(screen.getByRole("button", { name: "Save profile" }).closest("form")!));
    expect(screen.getByRole("alert").textContent).toContain("8 to 15 digits");
    expect(screen.getByLabelText("Phone")).toHaveProperty("value", "17");
  });

  it("confirms a save", async () => {
    const action = vi.fn(async () => ({ saved: true, displayName: "Karma", phone: "17123456" }));
    render(<ProfileForm displayName="" phone="" action={action} />);
    await act(async () => fireEvent.submit(screen.getByRole("button", { name: "Save profile" }).closest("form")!));
    expect(screen.getByRole("status").textContent).toBe("Profile saved.");
  });
});
