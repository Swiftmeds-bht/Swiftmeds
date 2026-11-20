import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MedicineCombobox } from "./MedicineCombobox";

const suggestions = [
  { productId: "p1", displayName: "Napa 500mg (Paracetamol)" },
  { productId: "p2", displayName: "Napa Extra (Paracetamol)" },
];

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ suggestions }), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const input = () => screen.getByRole("combobox", { name: "Medicine name" });
const wait = (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });

async function typeAndWait(text: string) {
  fireEvent.change(input(), { target: { value: text } });
  await wait(300);
}

describe("MedicineCombobox", () => {
  it("starts with the given value and no list", () => {
    render(<MedicineCombobox defaultValue="ibuprofen" />);
    expect(input()).toHaveProperty("value", "ibuprofen");
    expect(input().getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("asks once, after the patient pauses typing", async () => {
    render(<MedicineCombobox />);
    fireEvent.change(input(), { target: { value: "pa" } });
    await wait(100);
    fireEvent.change(input(), { target: { value: "par" } });
    await wait(300);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toBe("/api/suggest?prefix=par");
  });

  it("does not ask for fewer than 2 characters", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("p");
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("shows suggestions as options and announces how many there are", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");

    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual([
      "Napa 500mg (Paracetamol)",
      "Napa Extra (Paracetamol)",
    ]);
    expect(input().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("status").textContent).toContain("2 suggestions available");
  });

  it("moves through the suggestions with the arrow keys and wraps around", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");
    const options = screen.getAllByRole("option");

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input().getAttribute("aria-activedescendant")).toBe(options[0].id);
    expect(options[0].getAttribute("aria-selected")).toBe("true");

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input().getAttribute("aria-activedescendant")).toBe(options[1].id);

    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input().getAttribute("aria-activedescendant")).toBe(options[0].id);

    fireEvent.keyDown(input(), { key: "ArrowUp" });
    expect(input().getAttribute("aria-activedescendant")).toBe(options[1].id);
  });

  it("Enter picks the highlighted suggestion instead of submitting the form", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");
    fireEvent.keyDown(input(), { key: "ArrowDown" });

    const notPrevented = fireEvent.keyDown(input(), { key: "Enter" });

    expect(notPrevented).toBe(false); // the browser's own Enter (submit) was cancelled
    expect(input()).toHaveProperty("value", "Napa 500mg");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("Enter submits the form when nothing is highlighted", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");

    expect(fireEvent.keyDown(input(), { key: "Enter" })).toBe(true);
  });

  it("Escape closes the list and keeps what was typed", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");

    fireEvent.keyDown(input(), { key: "Escape" });

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input()).toHaveProperty("value", "nap");
  });

  it("picks a suggestion with the mouse", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");

    fireEvent.mouseDown(screen.getByRole("option", { name: "Napa Extra (Paracetamol)" }));

    expect(input()).toHaveProperty("value", "Napa Extra");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("closes the list when the box loses focus", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");

    fireEvent.blur(input());

    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("hides the list again when the text becomes too short", async () => {
    render(<MedicineCombobox />);
    await typeAndWait("nap");
    expect(screen.getByRole("listbox")).toBeTruthy();

    await typeAndWait("n");

    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("shows no list, and does not break, when the suggestion request fails", async () => {
    fetchMock.mockRejectedValue(new TypeError("fetch failed"));
    render(<MedicineCombobox />);

    await typeAndWait("nap");

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input()).toHaveProperty("value", "nap");
  });

  it("shows no list when the route answers with an error", async () => {
    fetchMock.mockResolvedValue(new Response("nope", { status: 500 }));
    render(<MedicineCombobox />);

    await typeAndWait("nap");

    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("shows no list when there are no suggestions", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ suggestions: [] }), { status: 200 }));
    render(<MedicineCombobox />);

    await typeAndWait("zzz");

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().getAttribute("aria-expanded")).toBe("false");
  });

  it("ignores the arrow keys when there is nothing to choose from", () => {
    render(<MedicineCombobox />);
    expect(fireEvent.keyDown(input(), { key: "ArrowDown" })).toBe(true);
    expect(fireEvent.keyDown(input(), { key: "ArrowUp" })).toBe(true);
  });
});
