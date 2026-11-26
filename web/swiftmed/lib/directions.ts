// Getting to the pharmacy (SCRUM-158).
import type { GeoPoint } from "./types";

const ok = (p: GeoPoint | undefined): p is GeoPoint =>
  !!p && Number.isFinite(p.latitude) && Number.isFinite(p.longitude) && Math.abs(p.latitude) <= 90 && Math.abs(p.longitude) <= 180;

/**
 * Directions to a pharmacy: Google Maps' universal link, which opens the maps
 * app on a phone and the website elsewhere, from the device's own location.
 * Undefined without a usable location.
 */
export function directionsUrl(to: GeoPoint | undefined): string | undefined {
  if (!ok(to)) return undefined;
  return `https://www.google.com/maps/dir/?api=1&destination=${to.latitude},${to.longitude}&travelmode=driving`;
}

/** The pharmacy on OpenStreetMap, for people who prefer it. */
export function openStreetMapUrl(at: GeoPoint | undefined): string | undefined {
  if (!ok(at)) return undefined;
  return `https://www.openstreetmap.org/?mlat=${at.latitude}&mlon=${at.longitude}#map=17/${at.latitude}/${at.longitude}`;
}
