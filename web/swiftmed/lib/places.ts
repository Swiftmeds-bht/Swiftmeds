import type { GeoPoint } from "./types";

/** Bhutan's regions, the groups of the place list. */
export const REGIONS = ["Western", "Southern", "Central", "Eastern"] as const;
export type Region = (typeof REGIONS)[number];

/**
 * Where the patient searches from, when not using the device's location: one
 * place per dzongkhag (SCRUM-153), at its main town. Pharmacy sign-up uses the
 * same list (lib/pharmacy-form.ts), so every dzongkhag a pharmacy can register
 * in can be searched. The ids are in search links, so they stay stable: the
 * first three are the original towns. `dzongkhag` is spelled as in the seed data
 * ("Chukha"), which Analytics groups by.
 */
export const PLACES = [
  { id: "thimphu", label: "Thimphu", dzongkhag: "Thimphu", region: "Western", location: { latitude: 27.4728, longitude: 89.639 } },
  { id: "paro", label: "Paro", dzongkhag: "Paro", region: "Western", location: { latitude: 27.4305, longitude: 89.4133 } },
  { id: "phuentsholing", label: "Phuentsholing (Chukha)", dzongkhag: "Chukha", region: "Southern", location: { latitude: 26.8516, longitude: 89.3884 } },
  { id: "haa", label: "Haa", dzongkhag: "Haa", region: "Western", location: { latitude: 27.3685, longitude: 89.2918 } },
  { id: "punakha", label: "Punakha", dzongkhag: "Punakha", region: "Western", location: { latitude: 27.5914, longitude: 89.8774 } },
  { id: "wangdue-phodrang", label: "Wangdue Phodrang", dzongkhag: "Wangdue Phodrang", region: "Western", location: { latitude: 27.4863, longitude: 89.8993 } },
  { id: "gasa", label: "Gasa", dzongkhag: "Gasa", region: "Western", location: { latitude: 27.9061, longitude: 89.7269 } },
  { id: "samtse", label: "Samtse", dzongkhag: "Samtse", region: "Southern", location: { latitude: 26.8993, longitude: 89.0995 } },
  { id: "sarpang", label: "Sarpang", dzongkhag: "Sarpang", region: "Southern", location: { latitude: 26.8639, longitude: 90.2674 } },
  { id: "dagana", label: "Dagana", dzongkhag: "Dagana", region: "Southern", location: { latitude: 27.0697, longitude: 89.8771 } },
  { id: "tsirang", label: "Tsirang", dzongkhag: "Tsirang", region: "Southern", location: { latitude: 27.0219, longitude: 90.1229 } },
  { id: "bumthang", label: "Bumthang", dzongkhag: "Bumthang", region: "Central", location: { latitude: 27.5492, longitude: 90.7525 } },
  { id: "trongsa", label: "Trongsa", dzongkhag: "Trongsa", region: "Central", location: { latitude: 27.5026, longitude: 90.5071 } },
  { id: "zhemgang", label: "Zhemgang", dzongkhag: "Zhemgang", region: "Central", location: { latitude: 27.2169, longitude: 90.6579 } },
  { id: "mongar", label: "Mongar", dzongkhag: "Mongar", region: "Eastern", location: { latitude: 27.2747, longitude: 91.2396 } },
  { id: "trashigang", label: "Trashigang", dzongkhag: "Trashigang", region: "Eastern", location: { latitude: 27.3333, longitude: 91.5539 } },
  { id: "trashiyangtse", label: "Trashiyangtse", dzongkhag: "Trashiyangtse", region: "Eastern", location: { latitude: 27.6116, longitude: 91.498 } },
  { id: "lhuentse", label: "Lhuentse", dzongkhag: "Lhuentse", region: "Eastern", location: { latitude: 27.6679, longitude: 91.1839 } },
  { id: "pemagatshel", label: "Pemagatshel", dzongkhag: "Pemagatshel", region: "Eastern", location: { latitude: 27.0379, longitude: 91.4031 } },
  { id: "samdrup-jongkhar", label: "Samdrup Jongkhar", dzongkhag: "Samdrup Jongkhar", region: "Eastern", location: { latitude: 26.8007, longitude: 91.5051 } },
] as const satisfies readonly { id: string; label: string; dzongkhag: string; region: Region; location: GeoPoint }[];

export type Place = (typeof PLACES)[number];
export type PlaceId = Place["id"];

/** The place list by region, in REGIONS order, each sorted by name, for grouped dropdowns. */
export function placesByRegion(): { region: Region; places: Place[] }[] {
  return REGIONS.map((region) => ({
    region,
    places: PLACES.filter((p) => p.region === region).sort((a, b) => a.label.localeCompare(b.label)),
  }));
}

// Search accepts 500..50000 m (search.proto).
export const RADII_M = [1000, 2000, 5000, 10000, 50000] as const;
export const DEFAULT_RADIUS_M = 5000;

export function placeById(id: string | undefined) {
  return PLACES.find((p) => p.id === id) ?? PLACES[0];
}

export function parseRadius(value: string | undefined): number {
  const n = Number(value);
  return (RADII_M as readonly number[]).includes(n) ? n : DEFAULT_RADIUS_M;
}

export interface ResolvedLocation {
  point: GeoPoint;
  /** Used in headings, e.g. "Pharmacies within 5.0 km of Thimphu". */
  label: string;
  /** True when the coordinates came from the patient's device, not from the town list. */
  fromDevice: boolean;
}

/** Reads latitude and longitude from the URL. Undefined unless both are numbers in range. */
export function parseCoordinates(lat: string | undefined, lng: string | undefined): GeoPoint | undefined {
  const latText = lat?.trim() ?? "";
  const lngText = lng?.trim() ?? "";
  if (latText === "" || lngText === "") return undefined;
  const latitude = Number(latText);
  const longitude = Number(lngText);
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) return undefined;
  if (Math.abs(latitude) > 90 || Math.abs(longitude) > 180) return undefined;
  return { latitude, longitude };
}

/** The device location if the URL has a valid one, otherwise the chosen town. */
export function resolveLocation(
  lat: string | undefined,
  lng: string | undefined,
  placeId: string | undefined,
): ResolvedLocation {
  const point = parseCoordinates(lat, lng);
  if (point) return { point, label: "your location", fromDevice: true };
  const place = placeById(placeId);
  return { point: place.location, label: place.label, fromDevice: false };
}

/** About 110 m. Coordinates end up in the page address, so they are kept coarse on purpose. */
export function roundCoordinate(value: number): number {
  return Math.round(value * 1000) / 1000;
}
