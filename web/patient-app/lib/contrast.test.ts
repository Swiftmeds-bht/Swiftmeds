// axe cannot measure colour contrast in jsdom, so the design tokens in
// app/globals.css are checked here with the WCAG 2.1 formula instead.
import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const css = readFileSync(path.join(import.meta.dirname, "../app/globals.css"), "utf8");
const token = (name: string) => {
  const m = css.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`));
  if (!m) throw new Error(`token --${name} not found`);
  return m[1];
};

describe("WCAG 2.1 AA contrast of the colour tokens", () => {
  // [foreground, background, minimum ratio]
  const pairs: [string, string, number][] = [
    ["text", "bg", 4.5],
    ["text", "surface", 4.5],
    ["muted", "bg", 4.5],
    ["muted", "surface", 4.5],
    ["primary", "bg", 4.5], // links and outline button
    ["primary-hover", "bg", 4.5], // hovered links
    ["primary-text", "primary", 4.5], // button
    ["primary-text", "primary-hover", 4.5], // hovered button
    ["primary", "high-bg", 4.5], // hovered outline button
    ["high-text", "high-bg", 4.5],
    ["medium-text", "medium-bg", 4.5],
    ["low-text", "low-bg", 4.5],
    ["info-text", "info-bg", 4.5], // notice banner
    ["focus", "bg", 3], // focus outline (non-text)
    ["input-border", "bg", 3], // search bar and select borders (non-text)
  ];
  it.each(pairs)("%s on %s >= %s:1", (fg, bg, min) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(min);
  });

  it("matches known reference values", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrast("#777777", "#ffffff")).toBeCloseTo(4.48, 2);
  });
});
