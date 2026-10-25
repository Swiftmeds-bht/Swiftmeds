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
