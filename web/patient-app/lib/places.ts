import type { GeoPoint } from "./types";

// Where the patient is searching from. A real location picker comes later;
// for now the patient chooses a town.
export const PLACES = [
  { id: "thimphu", label: "Thimphu", location: { latitude: 27.4728, longitude: 89.639 } },
  { id: "paro", label: "Paro", location: { latitude: 27.4305, longitude: 89.4133 } },
  { id: "phuentsholing", label: "Phuentsholing", location: { latitude: 26.8516, longitude: 89.3884 } },
] as const satisfies readonly { id: string; label: string; location: GeoPoint }[];

export type PlaceId = (typeof PLACES)[number]["id"];

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
