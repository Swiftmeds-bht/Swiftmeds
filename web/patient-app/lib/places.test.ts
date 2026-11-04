import { describe, expect, it } from "vitest";

import { parseCoordinates, parseRadius, placeById, resolveLocation, roundCoordinate } from "./places";

describe("parseCoordinates", () => {
  it("reads valid coordinates, including zero and negatives", () => {
    expect(parseCoordinates("27.473", "89.639")).toEqual({ latitude: 27.473, longitude: 89.639 });
    expect(parseCoordinates("0", "0")).toEqual({ latitude: 0, longitude: 0 });
    expect(parseCoordinates("-33.9", "-70.6")).toEqual({ latitude: -33.9, longitude: -70.6 });
  });

  it("accepts the limits and rejects values just outside them", () => {
    expect(parseCoordinates("90", "180")).toBeDefined();
    expect(parseCoordinates("-90", "-180")).toBeDefined();
    expect(parseCoordinates("90.001", "0")).toBeUndefined();
    expect(parseCoordinates("0", "180.001")).toBeUndefined();
    expect(parseCoordinates("-90.001", "0")).toBeUndefined();
  });

  it.each([
    [undefined, "89.6"],
    ["27.4", undefined],
    ["", "89.6"],
    ["  ", "89.6"],
    ["abc", "89.6"],
    ["27.4", "NaN"],
    ["Infinity", "89.6"],
  ])("rejects missing or unreadable values: %j, %j", (lat, lng) => {
    expect(parseCoordinates(lat, lng)).toBeUndefined();
  });
});

describe("resolveLocation", () => {
  it("prefers a valid device location over the town", () => {
    expect(resolveLocation("27.473", "89.639", "paro")).toEqual({
      point: { latitude: 27.473, longitude: 89.639 },
      label: "your location",
      fromDevice: true,
    });
  });

  it("falls back to the chosen town, then to the first town", () => {
    expect(resolveLocation(undefined, undefined, "paro")).toMatchObject({ label: "Paro", fromDevice: false });
    expect(resolveLocation("999", "89.6", "paro")).toMatchObject({ label: "Paro", fromDevice: false });
    expect(resolveLocation(undefined, undefined, "nowhere")).toMatchObject({ label: "Thimphu", fromDevice: false });
  });
});

describe("roundCoordinate", () => {
  it("keeps three decimals (about 110 m)", () => {
    expect(roundCoordinate(27.472812)).toBe(27.473);
    expect(roundCoordinate(89.6394)).toBe(89.639);
    expect(roundCoordinate(-0.0004)).toBe(-0);
  });
});

describe("town and radius helpers", () => {
  it("finds a town or defaults to Thimphu", () => {
    expect(placeById("paro").label).toBe("Paro");
    expect(placeById(undefined).label).toBe("Thimphu");
  });

  it("accepts only the offered radii", () => {
    expect(parseRadius("2000")).toBe(2000);
    expect(parseRadius("123")).toBe(5000);
    expect(parseRadius(undefined)).toBe(5000);
  });
});
