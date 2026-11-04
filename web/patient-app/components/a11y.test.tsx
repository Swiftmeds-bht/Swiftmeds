import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { mockSearch } from "@/lib/api";

import { ResultsList } from "./ResultsList";
import { ResultsSkeleton } from "./ResultsSkeleton";
import { SearchForm } from "./SearchForm";
import { SearchProblem, type ProblemKind } from "./SearchProblem";
import { expectNoSeriousViolations } from "./test-utils";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

afterEach(cleanup);

const thimphu = { latitude: 27.4728, longitude: 89.639 };

describe("SearchForm", () => {
  it("has no serious accessibility violations", async () => {
    const { container } = render(<SearchForm />);
    await expectNoSeriousViolations(container);
  });

  it("labels every control so screen readers can name it", () => {
    render(<SearchForm query="paracetamol" />);
    expect(screen.getByRole("combobox", { name: "Medicine name" })).toHaveProperty("value", "paracetamol");
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

  it("has no serious accessibility violations with substitutes", async () => {
    const { container } = render(
      <ResultsList
        query="amoxicillin"
        results={[{ productId: "p1", brandName: "Brand", genericName: "Amoxicillin", strength: "250mg", offers: [] }]}
        substitutes={mockSearch({ query: "amoxicillin", location: thimphu, radiusM: 5000 }).results}
      />,
    );
    await expectNoSeriousViolations(container);
    expect(screen.getByRole("heading", { name: "Same medicine, other brands" })).toBeTruthy();
  });

  it("links each pharmacy to its detail page", () => {
    const { results } = mockSearch({ query: "ibuprofen", location: thimphu, radiusM: 5000 });
    render(<ResultsList query="ibuprofen" results={results} substitutes={[]} />);
    const link = screen.getByRole("link", { name: "Demo Pharmacy Thimphu 02" });
    expect(link.getAttribute("href")).toBe("/pharmacy/40000000-0000-4000-8000-000000000002");
  });

  it("lists pharmacies in ranked order and marks the best match", () => {
    const { results } = mockSearch({ query: "Demo Paracetamol 500", location: thimphu, radiusM: 5000 });
    expect(results[0].offers.length).toBeGreaterThan(1);
    render(<ResultsList query="paracetamol" results={results} substitutes={[]} />);
    const list = screen.getByRole("list", { name: /best match first/ });
    expect(list.tagName).toBe("OL");
    expect(screen.getAllByText("Best match")).toHaveLength(1);
  });
});

describe("loading and error states", () => {
  it("the loading state has no serious violations and announces itself", async () => {
    const { container } = render(<ResultsSkeleton />);
    await expectNoSeriousViolations(container);
    expect(screen.getByRole("status").textContent).toContain("Searching nearby pharmacies");
  });

  const kinds: ProblemKind[] = ["unreachable", "timeout", "server", "not_available", "bad_request", "unexpected"];
  it.each(kinds)("the %s error state has no serious violations", async (kind) => {
    const { container } = render(<SearchProblem kind={kind} retryHref="/results?q=x" />);
    await expectNoSeriousViolations(container);
    expect(screen.getByRole("alert")).toBeTruthy();
  });
});
