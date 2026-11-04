import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SearchForm } from "./SearchForm";
import { expectNoSeriousViolations } from "./test-utils";

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

type Success = (position: { coords: { latitude: number; longitude: number } }) => void;
type Failure = (error: { code: number }) => void;

function mockGeolocation(impl: (ok: Success, fail: Failure) => void) {
  Object.defineProperty(navigator, "geolocation", { value: { getCurrentPosition: impl }, configurable: true });
}

beforeEach(() => {
  push.mockClear();
  vi.stubGlobal("fetch", vi.fn()); // the search box asks for suggestions only when typed in
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(navigator, "geolocation");
  vi.unstubAllGlobals();
});

const button = (name: string) => screen.getByRole("button", { name });
const select = (label: string) => screen.getByLabelText(label) as HTMLSelectElement;
const form = () => screen.getByRole("search");
const hidden = (container: HTMLElement, name: string) =>
  container.querySelector<HTMLInputElement>(`input[type="hidden"][name="${name}"]`);

function type(text: string) {
  fireEvent.change(screen.getByRole("combobox", { name: "Medicine name" }), { target: { value: text } });
}

describe("SearchForm: choosing a town", () => {
  it("searches the chosen town and radius", () => {
    render(<SearchForm />);
    type("paracetamol");
    fireEvent.change(select("Near"), { target: { value: "paro" } });
    fireEvent.change(select("Within"), { target: { value: "2000" } });

    fireEvent.submit(form());

    expect(push).toHaveBeenCalledWith("/results?q=paracetamol&place=paro&radius=2000");
  });

  it("starts from the values of the current search", () => {
    render(<SearchForm query="ibuprofen" placeId="phuentsholing" radiusM={10000} />);
    expect(screen.getByRole("combobox", { name: "Medicine name" })).toHaveProperty("value", "ibuprofen");
    expect(select("Near").value).toBe("phuentsholing");
    expect(select("Within").value).toBe("10000");
  });

  it("hides the location button when the browser cannot share a location", () => {
    render(<SearchForm />);
    expect(screen.queryByRole("button", { name: "Use my location" })).toBeNull();
    expect(select("Near").disabled).toBe(false);
  });
});

describe("SearchForm: using the device location", () => {
  it("uses the device location, rounded, instead of the town", () => {
    mockGeolocation((ok) => ok({ coords: { latitude: 27.472812, longitude: 89.639377 } }));
    const { container } = render(<SearchForm />);

    fireEvent.click(button("Use my location"));

    expect(hidden(container, "lat")?.value).toBe("27.473");
    expect(hidden(container, "lng")?.value).toBe("89.639");
    expect(select("Near").disabled).toBe(true);
    expect(screen.getAllByRole("status").some((s) => s.textContent === "Using your location.")).toBe(true);

    type("paracetamol");
    fireEvent.submit(form());
    expect(push).toHaveBeenCalledWith("/results?q=paracetamol&lat=27.473&lng=89.639&radius=5000");
  });

  it("says it is finding the location while it waits, and cannot be clicked twice", () => {
    mockGeolocation(() => {}); // never answers
    render(<SearchForm />);

    fireEvent.click(button("Use my location"));

    expect(screen.getAllByRole("status").some((s) => s.textContent === "Finding your location…")).toBe(true);
    expect((button("Use my location") as HTMLButtonElement).disabled).toBe(true);
  });

  it.each([
    [1, "blocked"],
    [2, "not available"],
    [3, "took too long"],
  ])("explains error code %i and keeps the town list working", (code, words) => {
    mockGeolocation((_ok, fail) => fail({ code }));
    const { container } = render(<SearchForm />);

    fireEvent.click(button("Use my location"));

    const message = screen.getAllByRole("status").find((s) => s.textContent?.includes("Choose a town"));
    expect(message?.textContent).toContain(words);
    expect(select("Near").disabled).toBe(false);
    expect(hidden(container, "lat")).toBeNull();

    type("paracetamol");
    fireEvent.submit(form());
    expect(push).toHaveBeenCalledWith("/results?q=paracetamol&place=thimphu&radius=5000");
  });

  it("goes back to the town list with Use a town instead", () => {
    mockGeolocation((ok) => ok({ coords: { latitude: 27.4, longitude: 89.6 } }));
    const { container } = render(<SearchForm />);
    fireEvent.click(button("Use my location"));

    fireEvent.click(button("Use a town instead"));

    expect(select("Near").disabled).toBe(false);
    expect(hidden(container, "lat")).toBeNull();
    expect(screen.getByRole("button", { name: "Use my location" })).toBeTruthy();
  });

  it("shows the device location again when the results page was opened from one", () => {
    mockGeolocation(() => {});
    const { container } = render(<SearchForm query="napa" deviceLocation={{ latitude: 27.473, longitude: 89.639 }} />);

    expect(hidden(container, "lat")?.value).toBe("27.473");
    expect(select("Near").disabled).toBe(true);
    expect(screen.getAllByRole("status").some((s) => s.textContent === "Using your location.")).toBe(true);
  });
});

describe("SearchForm accessibility with location states", () => {
  it.each([
    ["found", (ok: Success) => ok({ coords: { latitude: 27.4, longitude: 89.6 } })],
    ["denied", (_ok: Success, fail: Failure) => fail({ code: 1 })],
  ])("has no serious violations after location is %s", async (_name, impl) => {
    mockGeolocation(impl);
    const { container } = render(<SearchForm />);
    fireEvent.click(button("Use my location"));
    await expectNoSeriousViolations(container);
  });
});
