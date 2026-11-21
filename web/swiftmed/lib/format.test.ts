import { describe, expect, it } from "vitest";

import { formatDistance, formatPrice, formatQuantity, formatRelativeTime, freshnessLabel } from "./format";

describe("formatRelativeTime", () => {
  const now = Date.parse("2026-11-19T12:00:00Z");
  it.each([
    ["2026-11-19T11:59:40Z", "just now"],
    ["2026-11-19T11:55:00Z", "5 minutes ago"],
    ["2026-11-19T09:00:00Z", "3 hours ago"],
    ["2026-11-18T10:00:00Z", "yesterday"],
    ["2026-11-12T12:00:00Z", "last week"],
    ["2026-09-01T12:00:00Z", "1 Sept"],
    ["not a date", ""],
  ])("%s reads as %s", (iso, text) => {
    expect(formatRelativeTime(iso, now)).toBe(text);
  });
});

describe("formatPrice", () => {
  it.each([
    ["4500", "Nu. 45.00"],
    [4505, "Nu. 45.05"],
    ["1", "Nu. 0.01"],
    ["150000", "Nu. 1,500.00"],
  ])("%s chhertum -> %s", (input, want) => {
    expect(formatPrice(input)).toBe(want);
  });

  it.each([["0"], [0], [undefined], ["-5"], ["abc"]])("%s means no price", (input) => {
    expect(formatPrice(input as string)).toBe("Price not listed");
  });
});

describe("formatDistance", () => {
  it.each([
    [0, "0 m"],
    [460, "460 m"],
    [999, "999 m"],
    [1000, "1.0 km"],
    [2349, "2.3 km"],
    [50000, "50.0 km"],
    [undefined, "0 m"],
  ])("%s -> %s", (m, want) => {
    expect(formatDistance(m)).toBe(want);
  });
});

describe("freshnessLabel boundaries", () => {
  it.each([
    [1, "high"],
    [0.75, "high"],
    [0.7499, "medium"],
    [0.4, "medium"],
    [0.3999, "low"],
    [0, "low"],
    [undefined, "low"],
  ])("score %s -> %s", (score, level) => {
    expect(freshnessLabel(score).level).toBe(level);
  });
});

describe("formatQuantity", () => {
  it("uses singular for one unit", () => {
    expect(formatQuantity(1)).toBe("1 unit");
    expect(formatQuantity(12)).toBe("12 units");
    expect(formatQuantity(0)).toBe("0 units");
  });
});
