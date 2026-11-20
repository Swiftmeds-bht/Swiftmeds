import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "@/lib/mock-data";
import type { Hold, HoldStatus, PharmacyOffer, ProductResult } from "@/lib/types";

import { AuthForm } from "./AuthForm";
import { Countdown } from "./Countdown";
import { HoldCard } from "./HoldCard";
import { ReserveForm } from "./ReserveForm";
import { ResultsList, reserveHref } from "./ResultsList";
import { expectNoSeriousViolations } from "./test-utils";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  refresh.mockReset();
});

const noop = async () => ({});
const now = Date.parse("2026-11-05T08:00:00Z");
const [product, pharmacy] = [MOCK_PRODUCTS[0], MOCK_PHARMACIES[0]];

function hold(status: HoldStatus, extra: Partial<Hold> = {}): Hold {
  return {
    holdId: "h-1",
    pharmacyId: pharmacy.pharmacyId,
    productId: product.productId,
    quantity: 2,
    status,
    createdAt: "2026-11-05T07:59:58Z",
    ...extra,
  };
}

const active = hold("HOLD_STATUS_ACTIVE", { pickupCode: "482913", expiresAt: "2026-11-05T08:30:00Z" });

describe("HoldCard", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    vi.setSystemTime(now);
  });

  it("names the medicine, the quantity and the pharmacy", () => {
    render(<HoldCard hold={active} cancelAction={noop} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(`${product.brandName} ${product.strength}`);
    expect(screen.getByText(new RegExp(`2 units · ${pharmacy.name}`))).toBeTruthy();
  });

  it("shows an active hold's pickup code, time left and a Cancel button", async () => {
    const { container } = render(<HoldCard hold={active} cancelAction={noop} />);
    // Drawn digit by digit (each digit animates on its own), spaced for the eye.
    expect(container.querySelector(".pickup-code")?.textContent).toBe("4 8 2 9 1 3");
    expect(screen.getByText("4, 8, 2, 9, 1, 3")).toBeTruthy(); // read digit by digit
    expect(screen.getByText("30:00")).toBeTruthy();
    expect(screen.getByText(/until 14:30/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel hold" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("Ready to collect");
    await expectNoSeriousViolations(container);
  });

  it("refreshes while pending and shows no code yet", () => {
    render(<HoldCard hold={hold("HOLD_STATUS_PENDING")} cancelAction={noop} />);
    expect(screen.getByRole("status").textContent).toBe("Checking stock");
    expect(screen.queryByText(/Pickup code/)).toBeNull();
    expect(screen.queryByRole("button", { name: /Cancel/ })).toBeNull();
    act(() => vi.advanceTimersByTime(2000));
    expect(refresh).toHaveBeenCalled();
  });

  it.each([
    ["HOLD_STATUS_REJECTED", { statusReason: "Only 1 left at this pharmacy." }, /could not hold it: Only 1 left/],
    ["HOLD_STATUS_EXPIRED", { expiresAt: "2026-11-05T08:30:00Z" }, /ran out at 14:30/],
    ["HOLD_STATUS_CANCELLED", {}, /You cancelled this hold/],
    ["HOLD_STATUS_COLLECTED", {}, /You collected this medicine/],
  ] as const)("explains a %s hold without a code or Cancel", async (status, extra, message) => {
    const { container } = render(<HoldCard hold={hold(status, { ...extra, pickupCode: "123456" })} cancelAction={noop} />);
    expect(screen.getByText(message)).toBeTruthy();
    expect(screen.queryByText(/Pickup code/)).toBeNull();
    expect(screen.queryByRole("button", { name: /Cancel/ })).toBeNull();
    await expectNoSeriousViolations(container);
  });

  it("as a list row, links to the hold and names it on the Cancel button", () => {
    render(<HoldCard hold={active} cancelAction={noop} variant="row" />);
    expect(screen.getByRole("heading", { level: 3 })).toBeTruthy();
    expect(screen.getByRole("link", { name: `${product.brandName} ${product.strength}` }).getAttribute("href")).toBe("/holds/h-1");
    expect(screen.getByRole("button", { name: `Cancel hold for ${product.brandName} ${product.strength}` })).toBeTruthy();
  });
});

describe("Countdown", () => {
  it("ticks down and reloads the page when the time is up", () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    vi.setSystemTime(now);
    render(<Countdown expiresAt="2026-11-05T08:00:03Z" />);
    expect(screen.getByText("0:03")).toBeTruthy();
    act(() => vi.advanceTimersByTime(1000));
    expect(screen.getByText("0:02")).toBeTruthy();
    act(() => vi.advanceTimersByTime(2000));
    expect(screen.getByText("0:00")).toBeTruthy();
    expect(refresh).toHaveBeenCalledOnce();
  });
});

describe("AuthForm", () => {
  it("labels every field and links to the other form, keeping next", async () => {
    const { container } = render(<AuthForm mode="register" action={noop} next="/reserve?product=p" />);
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("Password")).toBeTruthy();
    expect(screen.getByLabelText("Repeat password")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Log in" }).getAttribute("href")).toBe("/login?next=%2Freserve%3Fproduct%3Dp");
    await expectNoSeriousViolations(container);
  });

  it("asks only for email and password to log in", async () => {
    const { container } = render(<AuthForm mode="login" action={noop} />);
    expect(screen.queryByLabelText("Repeat password")).toBeNull();
    expect(screen.getByRole("button", { name: "Log in" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Sign up" }).getAttribute("href")).toBe("/register");
    await expectNoSeriousViolations(container);
  });

  it("offers no sign-up to pharmacy staff, whose accounts are made for them", () => {
    render(<AuthForm mode="login" action={noop} next="/pharmacy-portal" signUp={false} />);
    expect(screen.getByRole("button", { name: "Log in" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Sign up" })).toBeNull();
  });
});

describe("ReserveForm", () => {
  it("offers no more units than allowed", async () => {
    const { container } = render(<ReserveForm action={noop} pharmacyId="ph" productId="pr" maxQuantity={3} returnTo="/reserve" />);
    const options = [...(screen.getByLabelText("How many?") as HTMLSelectElement).options].map((o) => o.value);
    expect(options).toEqual(["1", "2", "3"]);
    await expectNoSeriousViolations(container);
  });

  it("always offers at least one unit", () => {
    render(<ReserveForm action={noop} pharmacyId="ph" productId="pr" maxQuantity={0} returnTo="/reserve" />);
    expect((screen.getByLabelText("How many?") as HTMLSelectElement).options).toHaveLength(1);
  });

  it("shows the total to pay, and updates it with the number of units", () => {
    render(<ReserveForm action={noop} pharmacyId="ph" productId="pr" maxQuantity={5} returnTo="/reserve" unitPriceChhertum="1400" />);
    expect(screen.getByText("Nu. 14.00")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("How many?"), { target: { value: "3" } });
    expect(screen.getByText("Nu. 42.00")).toBeTruthy();
  });

  it("shows no total when the pharmacy listed no price", () => {
    render(<ReserveForm action={noop} pharmacyId="ph" productId="pr" maxQuantity={5} returnTo="/reserve" />);
    expect(screen.queryByText(/To pay at the counter/)).toBeNull();
  });
});

describe("Reserve link on a result", () => {
  const offer: PharmacyOffer = {
    pharmacyId: pharmacy.pharmacyId,
    pharmacyName: pharmacy.name,
    address: pharmacy.address,
    distanceM: 400,
    quantityAvailable: 3,
    priceChhertum: "1400",
    freshnessScore: 0.9,
    trustScore: 0.5,
    rankScore: 0,
  };
  const result: ProductResult = { ...product, offers: [offer] };

  it("carries the IDs and what the reserve page shows", () => {
    const url = new URL(reserveHref(result, offer), "http://x");
    expect(url.pathname).toBe("/reserve");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      product: product.productId,
      pharmacy: pharmacy.pharmacyId,
      name: `${product.brandName} ${product.strength}`,
      at: pharmacy.name,
      available: "3",
      price: "1400",
    });
    expect(new URL(reserveHref(result, { ...offer, priceChhertum: "0" }), "http://x").searchParams.has("price")).toBe(false);
  });

  it("is named after the pharmacy for screen readers", () => {
    render(<ResultsList query="para" results={[result]} substitutes={[]} />);
    expect(screen.getByRole("link", { name: `Reserve at ${pharmacy.name}` }).getAttribute("href")).toBe(reserveHref(result, offer));
  });
});
