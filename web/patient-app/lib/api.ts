// API client for the patient app. Calls the REST gateway at GATEWAY_URL; until
// the gateway exposes search (SCRUM-49) it falls back to demo data, and says so.
import { MOCK_PHARMACIES, MOCK_PRODUCTS, MOCK_STOCK } from "./mock-data";
import type { GeoPoint, PharmacyOffer, PharmacySummary, ProductResult, SearchParams, SearchResponse } from "./types";

export type DataSource = "gateway" | "demo";

export interface SearchResult {
  source: DataSource;
  data: SearchResponse;
}

export interface ApiOptions {
  /** Base URL of the gateway, e.g. http://localhost:8080. Empty means demo data only. */
  gatewayUrl?: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

const defaultOptions = (): ApiOptions => ({
  gatewayUrl: process.env.GATEWAY_URL ?? "",
  fetchFn: globalThis.fetch,
  timeoutMs: 3000,
});

/** Builds GET /api/v1/search?q=..&lat=..&lng=..&radius_m=.. */
export function searchUrl(gatewayUrl: string, p: SearchParams): string {
  const url = new URL("/api/v1/search", gatewayUrl);
  url.searchParams.set("q", p.query);
  url.searchParams.set("lat", String(p.location.latitude));
  url.searchParams.set("lng", String(p.location.longitude));
  url.searchParams.set("radius_m", String(p.radiusM));
  return url.toString();
}

export async function searchMedicines(params: SearchParams, options: ApiOptions = {}): Promise<SearchResult> {
  const opts = { ...defaultOptions(), ...options };
  if (opts.gatewayUrl && opts.fetchFn) {
    try {
      const res = await opts.fetchFn(searchUrl(opts.gatewayUrl, params), {
        headers: { Accept: "application/json" },
        cache: "no-store",
        signal: AbortSignal.timeout(opts.timeoutMs ?? 3000),
      });
      if (res.ok) {
        const body = (await res.json()) as Partial<SearchResponse>;
        return {
          source: "gateway",
          data: { searchId: body.searchId ?? "", results: body.results ?? [], substitutes: body.substitutes ?? [] },
        };
      }
      // 404 until the gateway route exists; anything else is also a reason to fall back.
    } catch {
      // Gateway unreachable or timed out: fall back to demo data below.
    }
  }
  return { source: "demo", data: mockSearch(params) };
}

export function getPharmacy(id: string): PharmacySummary | undefined {
  return MOCK_PHARMACIES.find((p) => p.pharmacyId === id);
}

// ---------------------------------------------------------------------------
// Demo search: same rules as the real Search service, on the seed data.

const EARTH_RADIUS_M = 6_371_008.8;

export function distanceM(a: GeoPoint, b: GeoPoint): number {
  const rad = (d: number) => (d * Math.PI) / 180;
  const dLat = rad(b.latitude - a.latitude);
  const dLng = rad(b.longitude - a.longitude);
  const h =
    Math.sin(dLat / 2) ** 2 + Math.cos(rad(a.latitude)) * Math.cos(rad(b.latitude)) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(h)));
}

function offersFor(productId: string, params: SearchParams): PharmacyOffer[] {
  const offers: PharmacyOffer[] = [];
  for (const s of MOCK_STOCK) {
    if (s.productId !== productId || s.quantity <= 0) continue;
    const pharmacy = getPharmacy(s.pharmacyId);
    if (!pharmacy) continue;
    const d = distanceM(params.location, pharmacy.location);
    if (d > params.radiusM) continue;
    offers.push({
      pharmacyId: pharmacy.pharmacyId,
      pharmacyName: pharmacy.name,
      address: pharmacy.address,
      location: pharmacy.location,
      distanceM: Math.round(d),
      quantityAvailable: s.quantity,
      priceChhertum: String(s.priceChhertum),
      freshnessScore: s.freshness,
      trustScore: 0.5,
      rankScore: 0,
    });
  }
  // Nearest first for the demo; the real order comes from the Ranking service.
  return offers.sort((a, b) => a.distanceM - b.distanceM || a.pharmacyId.localeCompare(b.pharmacyId));
}

function toResult(p: (typeof MOCK_PRODUCTS)[number], params: SearchParams): ProductResult {
  return {
    productId: p.productId,
    brandName: p.brandName,
    genericName: p.genericName,
    strength: p.strength,
    offers: offersFor(p.productId, params),
  };
}

export function mockSearch(params: SearchParams): SearchResponse {
  const q = params.query.trim().toLowerCase();
  if (!q) return { searchId: "demo", results: [], substitutes: [] };

  const matches = MOCK_PRODUCTS.filter(
    (p) => p.brandName.toLowerCase().includes(q) || p.genericName.toLowerCase().includes(q),
  );
  const results = matches.map((p) => toResult(p, params));

  // Like Search: suggest same-molecule, same-strength products only when none of
  // the matches is in stock nearby.
  let substitutes: ProductResult[] = [];
  if (results.length > 0 && results.every((r) => r.offers.length === 0)) {
    const wanted = new Set(matches.map((m) => `${m.moleculeId}|${m.strength}`));
    const matchedIds = new Set(matches.map((m) => m.productId));
    substitutes = MOCK_PRODUCTS.filter((p) => wanted.has(`${p.moleculeId}|${p.strength}`) && !matchedIds.has(p.productId))
      .map((p) => toResult(p, params))
      .filter((r) => r.offers.length > 0);
  }
  return { searchId: "demo", results, substitutes };
}
