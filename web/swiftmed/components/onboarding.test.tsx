import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Pharmacy } from "@/lib/pharmacies";

import { ReviewActions } from "./admin/ReviewActions";
import { MainNav } from "./MainNav";
import { PharmacySignupForm } from "./PharmacySignupForm";
import { PharmacyStatusPanel } from "./portal/PharmacyStatusPanel";
import { SignUpMenu } from "./SignUpMenu";
import { expectNoSeriousViolations } from "./test-utils";

vi.mock("next/navigation", () => ({ usePathname: () => "/admin", useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }) }));

afterEach(cleanup);
const noop = vi.fn(async () => ({}));

describe("SignUpMenu", () => {
  it("offers patients an account and pharmacies registration", async () => {
    const { container } = render(<SignUpMenu />);
    const button = screen.getByRole("button", { name: "Sign up" });
    expect(screen.queryByRole("region", { name: "Sign up" })).toBeNull();
    fireEvent.click(button);
    expect(screen.getByRole("link", { name: "Create an account" }).getAttribute("href")).toBe("/register");
    expect(screen.getByRole("link", { name: "Register your pharmacy" }).getAttribute("href")).toBe("/register/pharmacy");
    await expectNoSeriousViolations(container);
  });
});

describe("MainNav for admins", () => {
  it("shows the review page", () => {
    render(<MainNav audience="admin" />);
    const link = screen.getByRole("link", { name: "Pharmacies" });
    expect(link.getAttribute("href")).toBe("/admin");
    expect(link.getAttribute("aria-current")).toBe("page");
  });
});

describe("PharmacySignupForm", () => {
  it("asks for the login and the pharmacy, every field labelled", async () => {
    const { container } = render(<PharmacySignupForm action={noop} withAccount />);
    for (const label of ["Email", "Password", "Repeat password", "Pharmacy name", "Licence number", "Phone", "Street address", "Dzongkhag"]) {
      expect(screen.getByLabelText(label)).toBeTruthy();
    }
    expect(screen.getByRole("group", { name: "Your login" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Register pharmacy" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Log in to the pharmacy portal" }).getAttribute("href")).toBe("/login?next=%2Fpharmacy-portal");
    await expectNoSeriousViolations(container);
  });

  it("only asks for the pharmacy when an account is already signed in", () => {
    render(<PharmacySignupForm action={noop} withAccount={false} />);
    expect(screen.queryByLabelText("Email")).toBeNull();
    expect(screen.queryByRole("group", { name: "Your login" })).toBeNull();
    expect(screen.getByLabelText("Pharmacy name")).toBeTruthy();
  });

  it("uses the device's location when it is shared, and can go back to the dzongkhag's centre", () => {
    const getCurrentPosition = vi.fn((ok: PositionCallback) => ok({ coords: { latitude: 27.48012345, longitude: 89.64123456 } } as GeolocationPosition));
    Object.defineProperty(navigator, "geolocation", { value: { getCurrentPosition }, configurable: true });
    const { container } = render(<PharmacySignupForm action={noop} withAccount={false} />);

    fireEvent.click(screen.getByRole("button", { name: "Use my current location" }));
    expect(screen.getByRole("status").textContent).toContain("27.4801, 89.6412");
    expect((container.querySelector("input[name=latitude]") as HTMLInputElement).value).toBe("27.480123");

    fireEvent.click(screen.getByRole("button", { name: /dzongkhag's centre instead/ }));
    expect((container.querySelector("input[name=latitude]") as HTMLInputElement).value).toBe("");
  });
});

const base: Pharmacy = {
  pharmacyId: "ph-1",
  name: "Thimphu Care",
  status: "VERIFICATION_STATUS_PENDING",
  dzongkhag: "Thimphu",
  address: "Norzin Lam",
  createdAt: "2026-11-20T08:00:00Z",
};

describe("PharmacyStatusPanel", () => {
  it("tells a pending pharmacy it is waiting, with the steps", async () => {
    const { container } = render(<PharmacyStatusPanel pharmacy={base} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Waiting for approval");
    expect(container.querySelector(".review-step.done")?.textContent).toContain("20 November 2026");
    expect(container.querySelector("[aria-current=step]")?.textContent).toContain("Licence and details checked");
    await expectNoSeriousViolations(container);
  });

  it("gives a rejected pharmacy the reason", () => {
    render(<PharmacyStatusPanel pharmacy={{ ...base, status: "VERIFICATION_STATUS_REJECTED", statusReason: "Licence number not found." }} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Your registration was not approved");
    expect(screen.getByText("Licence number not found.")).toBeTruthy();
  });

  it("tells a suspended pharmacy it is hidden", () => {
    render(<PharmacyStatusPanel pharmacy={{ ...base, status: "VERIFICATION_STATUS_SUSPENDED", statusReason: "Licence expired." }} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Your pharmacy is suspended");
  });
});

describe("ReviewActions", () => {
  it("approves in one click and asks for a reason to reject", async () => {
    const { container } = render(<ReviewActions action={noop} pharmacyId="ph-1" name="Thimphu Care" tab="pending" allowed="review" />);
    expect(screen.getByRole("button", { name: "Approve Thimphu Care" })).toBeTruthy();
    fireEvent.click(screen.getByText("Reject…"));
    const reason = screen.getByLabelText("Reason (the pharmacy sees it)") as HTMLTextAreaElement;
    expect(reason.required).toBe(true);
    expect(screen.getByRole("button", { name: "Reject pharmacy" })).toBeTruthy();
    expect(screen.queryByText("Suspend…")).toBeNull();
    await expectNoSeriousViolations(container);
  });

  it("offers only Suspend for an approved pharmacy", () => {
    render(<ReviewActions action={noop} pharmacyId="ph-1" name="Thimphu Care" tab="verified" allowed="suspend" />);
    expect(screen.queryByRole("button", { name: /Approve/ })).toBeNull();
    expect(screen.getByText("Suspend…")).toBeTruthy();
  });
});
