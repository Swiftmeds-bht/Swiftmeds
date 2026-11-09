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
    ["text", "canvas", 4.5], // page text on the page ground
    ["text", "surface", 4.5],
    ["muted", "bg", 4.5],
    ["muted", "canvas", 4.5],
    ["muted", "surface", 4.5], // disabled select
    ["primary", "bg", 4.5], // links and outline button
    ["primary", "canvas", 4.5], // links on the page ground
    ["primary-hover", "bg", 4.5], // hovered links
    ["primary-text", "primary", 4.5], // button
    ["primary-text", "primary-hover", 4.5], // hovered button
    ["primary-hover", "primary-soft", 4.5], // hovered outline button, strength chip, Best match
    ["text", "primary-soft", 4.5], // best-match row, note under the steps
    ["muted", "primary-soft", 4.5], // address in the best-match row
    ["band-text", "band", 4.5], // home headline
    ["band-muted", "band", 4.5], // home lead and example caption
    ["ticket-text", "ticket", 4.5], // pickup code and countdown
    ["ticket-muted", "ticket", 4.5], // "Pickup code" and "left, until 14:30"
    ["high-text", "high-bg", 4.5],
    ["medium-text", "medium-bg", 4.5],
    ["low-text", "low-bg", 4.5],
    ["high-text", "bg", 4.5], // freshness on a result
    ["medium-text", "bg", 4.5],
    ["low-text", "bg", 4.5], // freshness, Cancel hold button
    ["high-text", "primary-soft", 4.5], // freshness in the best-match row
    ["medium-text", "primary-soft", 4.5],
    ["low-text", "primary-soft", 4.5],
    ["info-text", "info-bg", 4.5], // notice banner
    ["focus", "bg", 3], // focus outline (non-text)
    ["focus", "canvas", 3], // focus outline on the page ground (non-text)
    ["focus-on-band", "band", 3], // focus outline on the home band (non-text)
    ["input-border", "bg", 3], // field and select borders (non-text)
  ];
  it.each(pairs)("%s on %s >= %s:1", (fg, bg, min) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(min);
  });

  it("matches known reference values", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrast("#777777", "#ffffff")).toBeCloseTo(4.48, 2);
  });
});
