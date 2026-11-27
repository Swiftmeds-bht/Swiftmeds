import { describe, expect, it } from "vitest";

import { directionsUrl, openStreetMapUrl } from "./directions";
import { mapPins } from "./map-points";
import type { ProductResult } from "./types";

describe("directionsUrl (SCRUM-158)", () => {
  it("links to directions from the device to the pharmacy", () => {
    expect(directionsUrl({ latitude: 27.4728, longitude: 89.639 })).toBe(
      "https://www.google.com/maps/dir/?api=1&destination=27.4728,89.639&travelmode=driving",
    );
    expect(openStreetMapUrl({ latitude: 27.4728, longitude: 89.639 })).toContain("mlat=27.4728&mlon=89.639");
  });

  it("gives nothing without a usable location", () => {
    expect(directionsUrl(undefined)).toBeUndefined();
    expect(directionsUrl({ latitude: Number.NaN, longitude: 89 })).toBeUndefined();
    expect(directionsUrl({ latitude: 95, longitude: 89 })).toBeUndefined();
  });
});

const offer = (pharmacyId: string, name: string, distanceM: number, location?: { latitude: number; longitude: number }) => ({
  pharmacyId, pharmacyName: name, address: `${name} street`, location, distanceM, quantityAvailable: 4,
  freshnessScore: 1, trustScore: 1, rankScore: 1,
});

const results: ProductResult[] = [
  { productId: "p-1", brandName: "Napa", genericName: "Paracetamol", strength: "500mg",
    offers: [offer("ph-2", "Far Pharmacy", 2400, { latitude: 27.48, longitude: 89.64 }), offer("ph-1", "Near Pharmacy", 300, { latitude: 27.473, longitude: 89.639 })] },
  { productId: "p-2", brandName: "Ace", genericName: "Paracetamol", strength: "650mg",
    offers: [offer("ph-1", "Near Pharmacy", 300, { latitude: 27.473, longitude: 89.639 }), offer("ph-3", "No Location", 100)] },
];

describe("mapPins (SCRUM-158)", () => {
  it("puts one pin per pharmacy with a location, nearest first, with every medicine it has", () => {
    const pins = mapPins(results);
    expect(pins.map((p) => p.pharmacyId)).toEqual(["ph-1", "ph-2"]);
    expect(pins[0].medicines.map((m) => m.label)).toEqual(["Napa 500mg", "Ace 650mg"]);
    expect(pins[0].medicines[0].reserveHref).toMatch(/^\/reserve\?product=p-1&pharmacy=ph-1/);
  });

  it("is empty when nothing was found", () => {
    expect(mapPins([])).toEqual([]);
  });
});
