import axe from "axe-core";
import { expect } from "vitest";

// Fails on serious or critical WCAG 2.1 A/AA violations. Colour contrast cannot
// be computed in jsdom; it is covered by lib/contrast.test.ts.
export async function expectNoSeriousViolations(container: HTMLElement) {
  const result = await axe.run(container, {
    runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
    // Needs a real <canvas>, which jsdom lacks; see lib/contrast.test.ts.
    rules: { "color-contrast": { enabled: false } },
  });
  const serious = result.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}
