import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, it, vi } from "vitest";

import { MedicineCombobox } from "./MedicineCombobox";
import { expectNoSeriousViolations } from "./test-utils";

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(
      async () =>
        new Response(JSON.stringify({ suggestions: [{ productId: "p1", displayName: "Napa 500mg (Paracetamol)" }] }), {
          status: 200,
        }),
    ),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Real timers here: axe needs them. The 250 ms pause before suggestions is real too.
describe("MedicineCombobox accessibility", () => {
  it("has no serious violations when closed", async () => {
    const { container } = render(<MedicineCombobox />);
    await expectNoSeriousViolations(container);
  });

  it("has no serious violations with the suggestion list open and an option highlighted", async () => {
    const { container } = render(<MedicineCombobox />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "nap" } });
    await screen.findByRole("listbox");
    fireEvent.keyDown(screen.getByRole("combobox"), { key: "ArrowDown" });

    await expectNoSeriousViolations(container);
  });
});
