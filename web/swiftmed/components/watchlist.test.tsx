import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { WatchState } from "@/app/actions";
import { mockSearch } from "@/lib/api";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "@/lib/mock-data";
import type { AlertEvent } from "@/lib/watchlist";

import { AlertListener } from "./AlertListener";
import { ResultsList } from "./ResultsList";
import { expectNoSeriousViolations } from "./test-utils";
import { UnwatchForm } from "./UnwatchForm";
import { WatchButton } from "./WatchButton";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

const thimphu = { latitude: 27.4728, longitude: 89.639 };
const watchProps = {
  productId: MOCK_PRODUCTS[0].productId,
  medicine: "Demo Paracetamol 500 500mg",
  point: thimphu,
  radiusM: 5000,
  returnTo: "/results?q=paracetamol",
};

/** Stands in for the browser's EventSource, so a test can push events. */
class FakeEventSource {
  static all: FakeEventSource[] = [];
  closed = false;
  private listeners = new Map<string, ((e: Event) => void)[]>();
  constructor(readonly url: string) {
    FakeEventSource.all.push(this);
  }
  addEventListener(type: string, fn: (e: Event) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }
  close() {
    this.closed = true;
  }
  emit(type: string, data?: unknown) {
    const event = data === undefined ? new Event(type) : new MessageEvent(type, { data: JSON.stringify(data) });
    act(() => this.listeners.get(type)?.forEach((fn) => fn(event)));
  }
}

const alert: AlertEvent = {
  subscriptionId: "s-1",
  productId: MOCK_PRODUCTS[0].productId,
  pharmacyId: MOCK_PHARMACIES[0].pharmacyId,
  quantityAvailable: 4,
  occurredAt: "2026-11-11T08:05:00Z",
  medicine: "Demo Paracetamol 500 500mg",
  pharmacy: MOCK_PHARMACIES[0].name,
};

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  FakeEventSource.all = [];
});

describe("WatchButton", () => {
  it("sends the medicine and the searched area, then confirms", async () => {
    const action = vi.fn<(s: WatchState, f: FormData) => Promise<WatchState>>(async () => ({ watching: true }));
    const { container } = render(<WatchButton action={action} {...watchProps} />);
    await expectNoSeriousViolations(container);

    const button = screen.getByRole("button", { name: "Notify me when it is back for Demo Paracetamol 500 500mg" });
    await act(async () => fireEvent.click(button));

    const form = action.mock.calls[0][1];
    expect(Object.fromEntries(form)).toEqual({
      productId: watchProps.productId,
      lat: "27.4728",
      lng: "89.639",
      radius: "5000",
      returnTo: "/results?q=paracetamol",
    });
    expect(screen.getByRole("status").textContent).toContain("We will tell you when Demo Paracetamol 500 500mg is back nearby");
    expect(screen.getByRole("link", { name: "See your watchlist" }).getAttribute("href")).toBe("/watchlist");
  });

  it("shows the action's error", async () => {
    const action = async (): Promise<WatchState> => ({ error: "Could not add it." });
    render(<WatchButton action={action} {...watchProps} />);
    await act(async () => fireEvent.click(screen.getByRole("button")));
    expect(screen.getByRole("alert").textContent).toBe("Could not add it.");
  });
});

describe("ResultsList with a watchlist", () => {
  it("offers Notify me only on medicines nobody nearby has", async () => {
    const data = mockSearch({ query: "paracetamol", location: thimphu, radiusM: 5000 });
    const results = data.results.map((r, i) => (i === 0 ? { ...r, offers: [] } : r));
    const action = async (): Promise<WatchState> => ({});
    const { container } = render(
      <ResultsList query="paracetamol" results={results} substitutes={[]} watch={{ action, point: thimphu, radiusM: 5000, returnTo: "/results" }} />,
    );
    const buttons = screen.getAllByRole("button", { name: /Notify me when it is back/ });
    expect(buttons).toHaveLength(results.filter((r) => r.offers.length === 0).length);
    await expectNoSeriousViolations(container);
  });

  it("has no Notify me button without a watchlist", () => {
    const data = mockSearch({ query: "paracetamol", location: thimphu, radiusM: 5000 });
    render(<ResultsList query="paracetamol" results={data.results.map((r) => ({ ...r, offers: [] }))} substitutes={[]} />);
    expect(screen.queryByRole("button", { name: /Notify me/ })).toBeNull();
  });
});

describe("UnwatchForm", () => {
  it("names the medicine it removes", async () => {
    const action = vi.fn<(s: object, f: FormData) => Promise<object>>(async () => ({}));
    const { container } = render(<UnwatchForm action={action} subscriptionId="s-1" medicine="Demo Ibuprofen 400 400mg" />);
    await expectNoSeriousViolations(container);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove Demo Ibuprofen 400 400mg from the watchlist" })));
    expect(Object.fromEntries(action.mock.calls[0][1])).toEqual({ subscriptionId: "s-1" });
  });
});

describe("AlertListener", () => {
  beforeEach(() => vi.stubGlobal("EventSource", FakeEventSource));

  it("shows an alert as soon as it arrives, without a refresh", async () => {
    const { container } = render(<AlertListener />);
    expect(container.textContent).toBe("");
    const [source] = FakeEventSource.all;
    expect(source.url).toBe("/api/alerts");

    source.emit("alert", alert);
    source.emit("alert", alert); // the same alert twice shows once
    expect(screen.getByText("Demo Paracetamol 500 500mg is back in stock")).toBeTruthy();
    expect(screen.getByText(`4 units at ${MOCK_PHARMACIES[0].name}.`)).toBeTruthy();
    expect(screen.getAllByText(/is back in stock/)).toHaveLength(1);
    expect(refresh).toHaveBeenCalled(); // the page's own data catches up too
    expect(screen.getByRole("link", { name: "See pharmacy" }).getAttribute("href")).toBe(`/pharmacy/${MOCK_PHARMACIES[0].pharmacyId}`);
    await expectNoSeriousViolations(container);

    fireEvent.click(screen.getByRole("button", { name: /Dismiss/ }));
    expect(screen.queryByText(/is back in stock/)).toBeNull();
  });

  it("reconnects after a dropped connection, but not once signed out", () => {
    vi.useFakeTimers();
    render(<AlertListener />);
    FakeEventSource.all[0].emit("error");
    expect(FakeEventSource.all[0].closed).toBe(true);

    act(() => vi.advanceTimersByTime(2000));
    expect(FakeEventSource.all).toHaveLength(2);

    FakeEventSource.all[1].emit("error", { kind: "unauthorized" });
    act(() => vi.advanceTimersByTime(120_000));
    expect(FakeEventSource.all).toHaveLength(2);
  });

  it("closes the connection when the page goes away", () => {
    const { unmount } = render(<AlertListener />);
    unmount();
    expect(FakeEventSource.all[0].closed).toBe(true);
  });
});
