import { describe, expect, it } from "vitest";

import { chhertumToNu, parsePriceNu } from "./prices";

describe("parsePriceNu (SCRUM-154)", () => {
  it.each([
    ["45", 4500],
    ["45.5", 4550],
    ["45.50", 4550],
    ["0.05", 5],
    [" Nu. 1,250.75 ", 125075],
    ["1000000", 100_000_000],
  ])("reads %j as %i chhertum", (text, chhertum) => {
    expect(parsePriceNu(text)).toEqual({ chhertum });
  });

  it.each([
    ["", "ngultrum"],
    ["abc", "ngultrum"],
    ["45.505", "ngultrum"],
    ["-5", "ngultrum"],
    ["0", "more than Nu. 0"],
    ["0.00", "more than Nu. 0"],
    ["1000000.01", "at most"],
  ])("refuses %j", (text, words) => {
    const result = parsePriceNu(text);
    expect("error" in result && result.error).toContain(words);
  });

  it("shows chhertum as ngultrum with two decimals", () => {
    expect(chhertumToNu(4550)).toBe("45.50");
    expect(chhertumToNu(5)).toBe("0.05");
  });
});
