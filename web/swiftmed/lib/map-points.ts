// The pharmacies to put on the results map (SCRUM-158): one pin per pharmacy,
// with every medicine it has from this search.
import { reserveHref } from "@/components/ResultsList";

import type { GeoPoint, ProductResult } from "./types";

export interface MapPin {
  pharmacyId: string;
  name: string;
  address: string;
  location: GeoPoint;
  distanceM: number;
  medicines: { label: string; reserveHref: string }[];
}

const productLabel = (p: ProductResult) => [p.brandName, p.strength].filter(Boolean).join(" ") || p.genericName;

/** Pins for the pharmacies with a location, nearest first. Substitutes are left out: the list shows them separately. */
export function mapPins(results: ProductResult[]): MapPin[] {
  const pins = new Map<string, MapPin>();
  for (const product of results) {
    for (const offer of product.offers) {
      const loc = offer.location;
      if (!loc || !Number.isFinite(loc.latitude) || !Number.isFinite(loc.longitude)) continue;
      let pin = pins.get(offer.pharmacyId);
      if (!pin) {
        pin = {
          pharmacyId: offer.pharmacyId,
          name: offer.pharmacyName,
          address: offer.address,
          location: loc,
          distanceM: offer.distanceM,
          medicines: [],
        };
        pins.set(offer.pharmacyId, pin);
      }
      pin.medicines.push({ label: productLabel(product), reserveHref: reserveHref(product, offer) });
    }
  }
  return [...pins.values()].sort((a, b) => a.distanceM - b.distanceM);
}
