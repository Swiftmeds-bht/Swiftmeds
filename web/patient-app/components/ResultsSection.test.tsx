import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "@/lib/api";

import { ResultsSection } from "./ResultsSection";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  searchMedicines: vi.fn(),
}));

const search = vi.mocked(api.searchMedicines);
const thimphu = { latitude: 27.4728, longitude: 89.639 };
const props = {
  query: "paracetamol",
  point: thimphu,
  label: "Thimphu",
  radiusM: 5000,
  retryHref: "/results?q=paracetamol&place=thimphu",
};

const found = (over: Partial<api.SearchResult> = {}): api.SearchResult => ({
  source: "gateway",
  data: api.mockSearch({ query: "paracetamol", location: thimphu, radiusM: 5000 }),
  degraded: [],
  ...over,
});

beforeEach(() => {
  search.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const show = async (p = props) => render(await ResultsSection(p));

describe("ResultsSection", () => {
  it("asks the search for the query, location and radius, and lists the pharmacies", async () => {
    search.mockResolvedValue(found());

    await show();

    expect(search).toHaveBeenCalledWith({ query: "paracetamol", location: thimphu, radiusM: 5000 });
    expect(screen.getByRole("heading", { name: "Pharmacies within 5.0 km of Thimphu" })).toBeTruthy();
    expect(screen.getByText(/pharmac(y has|ies have)/)).toBeTruthy();
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("says when it is showing demo data", async () => {
    search.mockResolvedValue(found({ source: "demo" }));
    await show();
    expect(screen.getByRole("note").textContent).toContain("demo data");
  });

  it("warns when optional details were unavailable", async () => {
    search.mockResolvedValue(found({ degraded: ["freshness", "ranking"] }));
    await show();
    expect(screen.getByRole("note").textContent).toContain("Some details are unavailable");
  });

  it("shows the empty state when nothing matches", async () => {
    search.mockResolvedValue(found({ data: { searchId: "s", results: [], substitutes: [] } }));
    await show();
    expect(screen.getByText(/No medicines match/)).toBeTruthy();
  });

  it.each([
    ["unreachable", /could not reach/i],
    ["timeout", /taking too long/i],
    ["server", /not working right now/i],
    ["not_available", /not available yet/i],
    ["bad_request", /not accepted/i],
  ] as const)("shows the %s error state with a way to try again", async (kind, title) => {
    search.mockRejectedValue(new api.SearchError(kind, "boom"));

    await show();

    expect(screen.getByRole("alert").textContent).toMatch(title);
    const retry = screen.queryByRole("link", { name: "Try again" });
    if (kind === "bad_request") expect(retry).toBeNull();
    else expect(retry?.getAttribute("href")).toBe(props.retryHref);
  });

  it("never shows demo or empty results in place of a failed search", async () => {
    search.mockRejectedValue(new api.SearchError("unreachable", "boom"));
    await show();
    expect(screen.queryByRole("note")).toBeNull();
    expect(screen.queryByText(/pharmac(y has|ies have)/)).toBeNull();
    expect(screen.queryByText(/No medicines match/)).toBeNull();
  });

  it.each([
    ["too short", "a"],
    ["too long", "x".repeat(101)],
  ])("rejects a query that is %s without calling the search", async (_name, query) => {
    await show({ ...props, query });

    expect(search).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/not accepted/i);
  });

  it("lets unexpected errors reach the error boundary", async () => {
    search.mockRejectedValue(new Error("boom"));
    await expect(ResultsSection(props)).rejects.toThrow("boom");
  });
});
