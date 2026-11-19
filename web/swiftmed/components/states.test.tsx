import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ResultsSkeleton } from "./ResultsSkeleton";
import { SearchProblem, type ProblemKind } from "./SearchProblem";

afterEach(cleanup);

describe("SearchProblem", () => {
  it("gives every kind of problem its own plain-language message", () => {
    const kinds: ProblemKind[] = ["unreachable", "timeout", "server", "not_available", "bad_request", "unexpected"];
    const titles = kinds.map((kind) => {
      const { unmount } = render(<SearchProblem kind={kind} />);
      const title = screen.getByRole("heading").textContent ?? "";
      unmount();
      return title;
    });
    expect(new Set(titles).size).toBe(kinds.length);
    expect(titles.every((t) => t.length > 0)).toBe(true);
  });

  it("offers a Try again link back to the same search", () => {
    render(<SearchProblem kind="server" retryHref="/results?q=napa&place=paro" />);
    expect(screen.getByRole("link", { name: "Try again" }).getAttribute("href")).toBe("/results?q=napa&place=paro");
  });

  it("does not offer Try again when the search itself was not valid", () => {
    render(<SearchProblem kind="bad_request" retryHref="/results?q=a" />);
    expect(screen.queryByRole("link", { name: "Try again" })).toBeNull();
    expect(screen.getByRole("link", { name: "start a new search" }).getAttribute("href")).toBe("/");
  });

  it("calls onRetry from the Try again button", () => {
    const onRetry = vi.fn();
    render(<SearchProblem kind="unexpected" onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});

describe("ResultsSkeleton", () => {
  it("tells screen readers that a search is running and hides the grey blocks", () => {
    const { container } = render(<ResultsSkeleton />);
    expect(screen.getByRole("status").textContent).toContain("Searching nearby pharmacies");
    expect(container.querySelectorAll('[aria-hidden="true"]').length).toBeGreaterThan(0);
  });
});
