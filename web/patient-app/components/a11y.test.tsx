import { cleanup, render, screen } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it } from "vitest";

import { mockSearch } from "@/lib/api";

import { ResultsList } from "./ResultsList";
import { SearchForm } from "./SearchForm";

afterEach(cleanup);

// Fails on serious or critical WCAG 2.1 A/AA violations. Colour contrast cannot
// be computed in jsdom; it is covered by lib/contrast.test.ts.
async function expectNoSeriousViolations(container: HTMLElement) {
  const result = await axe.run(container, {
    runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
    // Needs a real <canvas>, which jsdom lacks; see lib/contrast.test.ts.
    rules: { "color-contrast": { enabled: false } },
  });
  const serious = result.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}

const thimphu = { latitude: 27.4728, longitude: 89.639 };

describe("SearchForm", () => {
  it("has no serious accessibility violations", async () => {
    const { container } = render(<SearchForm />);
    await expectNoSeriousViolations(container);
  });

  it("labels every control so screen readers can name it", () => {
    render(<SearchForm query="paracetamol" />);
    expect(screen.getByLabelText("Medicine name")).toHaveProperty("value", "paracetamol");
    expect(screen.getByLabelText("Near")).toBeTruthy();
    expect(screen.getByLabelText("Within")).toBeTruthy();
    expect(screen.getByRole("search", { name: "Search for a medicine" })).toBeTruthy();
  });
});

describe("ResultsList", () => {
  it("has no serious accessibility violations with results", async () => {
    const { results, substitutes } = mockSearch({ query: "paracetamol", location: thimphu, radiusM: 5000 });
    const { container } = render(<ResultsList query="paracetamol" results={results} substitutes={substitutes} />);
    await expectNoSeriousViolations(container);
    expect(screen.getByRole("status").textContent).toMatch(/pharmac(y has|ies have)/);
  });

  it("has no serious accessibility violations when nothing matches", async () => {
    const { container } = render(<ResultsList query="nonexistium" results={[]} substitutes={[]} />);
    await expectNoSeriousViolations(container);
    expect(screen.getByRole("status").textContent).toContain("No medicines match");
  });

  it("links each pharmacy to its detail page", () => {
    const { results } = mockSearch({ query: "ibuprofen", location: thimphu, radiusM: 5000 });
    render(<ResultsList query="ibuprofen" results={results} substitutes={[]} />);
    const link = screen.getByRole("link", { name: "Demo Pharmacy Thimphu 02" });
    expect(link.getAttribute("href")).toBe("/pharmacy/40000000-0000-4000-8000-000000000002");
  });
});
