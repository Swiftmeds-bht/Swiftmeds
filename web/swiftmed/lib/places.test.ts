import { describe, expect, it } from "vitest";

import { DZONGKHAGS } from "./pharmacy-form";
import { PLACES, REGIONS, parseCoordinates, parseRadius, placeById, placesByRegion, resolveLocation, roundCoordinate } from "./places";

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

describe("the place list (SCRUM-153)", () => {
  it("has one place per dzongkhag, all 20, with unique ids", () => {
    expect(PLACES).toHaveLength(20);
    expect(new Set(PLACES.map((p) => p.dzongkhag)).size).toBe(20);
    expect(new Set(PLACES.map((p) => p.id)).size).toBe(20);
  });

  it("keeps the original ids, so old search links still work, and Thimphu first as the default", () => {
    expect(PLACES[0].id).toBe("thimphu");
    expect(placeById("paro").label).toBe("Paro");
    expect(placeById("phuentsholing")).toMatchObject({ dzongkhag: "Chukha", label: "Phuentsholing (Chukha)" });
    expect(placeById("mongar").location).toEqual({ latitude: 27.2747, longitude: 91.2396 });
    expect(placeById("nowhere").id).toBe("thimphu");
  });

  it("puts every place in Bhutan and in exactly one region", () => {
    for (const p of PLACES) {
      expect(p.location.latitude).toBeGreaterThan(26.6);
      expect(p.location.latitude).toBeLessThan(28.4);
      expect(p.location.longitude).toBeGreaterThan(88.7);
      expect(p.location.longitude).toBeLessThan(92.2);
    }
    const grouped = placesByRegion();
    expect(grouped.map((g) => g.region)).toEqual([...REGIONS]);
    expect(grouped.flatMap((g) => g.places)).toHaveLength(20);
    expect(grouped[0].places.map((p) => p.label)).toEqual(["Gasa", "Haa", "Paro", "Punakha", "Thimphu", "Wangdue Phodrang"]);
  });

  it("is the same list the pharmacy form registers in", () => {
    expect(DZONGKHAGS.map((d) => d.name).sort()).toEqual(PLACES.map((p) => p.dzongkhag).sort());
    expect(DZONGKHAGS.find((d) => d.name === "Chukha")?.centre).toEqual(placeById("phuentsholing").location);
  });

  it("resolves a new place for the results page", () => {
    expect(resolveLocation(undefined, undefined, "trashigang")).toEqual({
      point: { latitude: 27.3333, longitude: 91.5539 },
      label: "Trashigang",
      fromDevice: false,
    });
  });
});
