import { describe, expect, it } from "vitest";

import { MAX_QUERY_LENGTH, MIN_QUERY_LENGTH, suggestionQuery } from "./rules";

describe("suggestionQuery", () => {
  it.each([
    ["Napa 500mg (Paracetamol)", "Napa 500mg"],
    ["Brufen 400 (Ibuprofen)", "Brufen 400"],
    ["Paracetamol", "Paracetamol"],
    ["  Napa Extra  (Paracetamol)", "Napa Extra"],
    ["(Odd) name", "(Odd) name"],
  ])("turns %j into %j", (displayName, want) => {
    expect(suggestionQuery({ productId: "p", displayName })).toBe(want);
  });
});

describe("query limits", () => {
  it("match the Search service (2 to 100 characters)", () => {
    expect([MIN_QUERY_LENGTH, MAX_QUERY_LENGTH]).toEqual([2, 100]);
  });
});
