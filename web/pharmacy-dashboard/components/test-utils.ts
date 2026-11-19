import axe from "axe-core";
import { expect } from "vitest";

// Fails on serious or critical WCAG 2.1 A/AA violations, as in the patient app.
// Colour contrast cannot be computed in jsdom; the shared tokens are checked by
// web/patient-app/lib/contrast.test.ts.
export async function expectNoSeriousViolations(container: HTMLElement) {
  const result = await axe.run(container, {
    runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
    rules: { "color-contrast": { enabled: false } },
  });
  const serious = result.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}
