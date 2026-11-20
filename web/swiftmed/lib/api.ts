// API client for the patient app. It calls the REST gateway at GATEWAY_URL
// (default http://localhost:8080). Only with USE_DEMO_DATA=true does it serve
// clearly labelled demo data (lib/data-source.ts). When the gateway fails, it
// throws a SearchError: it never shows demo stock in place of a real search,
// because a patient could act on stock that is not there.
import { gatewayUrl } from "./data-source";
import { MOCK_PHARMACIES, MOCK_PRODUCTS, MOCK_STOCK } from "./mock-data";
import { MAX_QUERY_LENGTH, MIN_QUERY_LENGTH } from "./rules";
import type {
  GeoPoint,
  PharmacyDetails,
  PharmacyOffer,
  PharmacySummary,
  ProductResult,
  SearchParams,
  SearchResponse,
  Suggestion,
} from "./types";

export type DataSource = "gateway" | "demo";

export interface SearchResult {
  source: DataSource;
  data: SearchResponse;
  /** Optional services that did not answer (from the x-search-degraded header). Empty when all is well. */
  degraded: string[];
}

export interface ApiOptions {
  /** Base URL of the gateway, e.g. http://localhost:8080. Empty means demo data only. */
  gatewayUrl?: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

const defaultOptions = (): ApiOptions => ({
  gatewayUrl: gatewayUrl(),
  fetchFn: globalThis.fetch,
  timeoutMs: 5000,
});

export type SearchErrorKind = "unreachable" | "timeout" | "not_available" | "bad_request" | "server";

/** Why a gateway call failed, in terms the screen can explain to the patient. */
export class SearchError extends Error {
  readonly kind: SearchErrorKind;

  constructor(kind: SearchErrorKind, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "SearchError";
    this.kind = kind;
  }
}

function errorForStatus(status: number): SearchError {
  if (status === 400) return new SearchError("bad_request", "The gateway rejected the search (HTTP 400)");
  if (status === 404 || status === 501) {
    return new SearchError("not_available", `The gateway has no search route (HTTP ${status})`);
  }
  if (status === 408 || status === 504) return new SearchError("timeout", `The search timed out (HTTP ${status})`);
  return new SearchError("server", `The gateway could not search (HTTP ${status})`);
}

async function getJson(url: string, opts: ApiOptions): Promise<{ body: unknown; headers: Headers }> {
  let res: Response;
  try {
    res = await (opts.fetchFn ?? fetch)(url, {
      headers: { Accept: "application/json" },
      cache: "no-store",
      signal: AbortSignal.timeout(opts.timeoutMs ?? 5000),
    });
  } catch (cause) {
    // By name, not instanceof: a DOMException from another realm is not an instance of this realm's Error.
    const name = (cause as { name?: unknown } | null)?.name;
    const timedOut = name === "TimeoutError" || name === "AbortError";
    throw new SearchError(
      timedOut ? "timeout" : "unreachable",
      timedOut ? "The gateway did not answer in time" : "Could not reach the gateway",
      { cause },
    );
  }
  if (!res.ok) throw errorForStatus(res.status);
  try {
    return { body: await res.json(), headers: res.headers };
  } catch (cause) {
    throw new SearchError("server", "The gateway sent an answer that is not JSON", { cause });
  }
}

/** Builds GET /api/v1/search?q=..&lat=..&lng=..&radius_m=.. */
export function searchUrl(gatewayUrl: string, p: SearchParams): string {
  const url = new URL("/api/v1/search", gatewayUrl);
  url.searchParams.set("q", p.query);
  url.searchParams.set("lat", String(p.location.latitude));
  url.searchParams.set("lng", String(p.location.longitude));
  url.searchParams.set("radius_m", String(p.radiusM));
  return url.toString();
}

/** Builds GET /api/v1/search/autocomplete?prefix=..&limit=.. */
export function autocompleteUrl(gatewayUrl: string, prefix: string, limit = 8): string {
  const url = new URL("/api/v1/search/autocomplete", gatewayUrl);
  url.searchParams.set("prefix", prefix);
  url.searchParams.set("limit", String(limit));
  return url.toString();
}

/** Searches through the gateway, or on demo data when no gateway is configured. Throws SearchError. */
export async function searchMedicines(params: SearchParams, options: ApiOptions = {}): Promise<SearchResult> {
  const opts = { ...defaultOptions(), ...options };
  if (!opts.gatewayUrl) return { source: "demo", data: mockSearch(params), degraded: [] };

  const { body, headers } = await getJson(searchUrl(opts.gatewayUrl, params), opts);
  const data = (body ?? {}) as Partial<SearchResponse>;
  const degraded = (headers.get("x-search-degraded") ?? "")
    .split(",")
    .map((name) => name.trim())
    .filter(Boolean);
  return {
    source: "gateway",
    data: {
      searchId: data.searchId ?? "",
      results: (data.results ?? []).map(normalizeProduct),
      substitutes: (data.substitutes ?? []).map(normalizeProduct),
    },
    degraded,
  };
}

// Protobuf JSON leaves out fields that hold their default value: an empty list,
// 0 or "". A product nobody nearby stocks arrives without `offers`, and a
// pharmacy 0 m away without `distanceM`. Fill the defaults back in so the
// screens can rely on the types in lib/types.ts.
function normalizeProduct(p: Partial<ProductResult>): ProductResult {
  return {
    productId: p.productId ?? "",
    brandName: p.brandName ?? "",
    genericName: p.genericName ?? "",
    strength: p.strength ?? "",
    offers: (p.offers ?? []).map(normalizeOffer),
  };
}

function normalizeOffer(o: Partial<PharmacyOffer>): PharmacyOffer {
  return {
    ...o,
    pharmacyId: o.pharmacyId ?? "",
    pharmacyName: o.pharmacyName ?? "",
    address: o.address ?? "",
    distanceM: o.distanceM ?? 0,
    quantityAvailable: o.quantityAvailable ?? 0,
    freshnessScore: o.freshnessScore ?? 0,
    trustScore: o.trustScore ?? 0,
    rankScore: o.rankScore ?? 0,
  };
}

function isSuggestion(value: unknown): value is Suggestion {
  const v = value as Partial<Suggestion> | null;
  return typeof v?.productId === "string" && typeof v.displayName === "string";
}

/**
 * Name suggestions for the search box. Suggestions are a convenience, so any
 * failure gives an empty list instead of an error.
 */
export async function fetchSuggestions(prefix: string, options: ApiOptions = {}): Promise<Suggestion[]> {
  const text = prefix.trim();
  if (text.length < MIN_QUERY_LENGTH || text.length > MAX_QUERY_LENGTH) return [];

  const opts = { ...defaultOptions(), timeoutMs: 1500, ...options };
  if (!opts.gatewayUrl) return mockSuggest(text);
  try {
    const { body } = await getJson(autocompleteUrl(opts.gatewayUrl, text), opts);
    const list = (body as { suggestions?: unknown } | null)?.suggestions;
    return Array.isArray(list) ? list.filter(isSuggestion) : [];
  } catch {
    return [];
  }
}

/** A demo pharmacy from the seed data. */
export function getPharmacy(id: string): PharmacySummary | undefined {
  return MOCK_PHARMACIES.find((p) => p.pharmacyId === id);
}

export type PharmacyLookup =
  | { status: "found"; source: DataSource; pharmacy: PharmacyDetails }
  | { status: "not_found" }
  /** The gateway cannot give pharmacy details right now (or has no such route yet). Never replaced by demo data. */
  | { status: "unavailable"; kind: SearchErrorKind };

/** Builds GET /api/v1/pharmacies/{pharmacyId} */
export function pharmacyUrl(gatewayUrl: string, pharmacyId: string): string {
  return new URL(`/api/v1/pharmacies/${encodeURIComponent(pharmacyId)}`, gatewayUrl).toString();
}

/**
 * One pharmacy for the pharmacy page: from the gateway (openapi.yaml getPharmacy,
 * public) when GATEWAY_URL is set, else from the demo data. Only verified
 * pharmacies are shown to patients.
 */
export async function lookupPharmacy(pharmacyId: string, options: ApiOptions = {}): Promise<PharmacyLookup> {
  const opts = { ...defaultOptions(), ...options };
  if (!opts.gatewayUrl) {
    const p = getPharmacy(pharmacyId);
    return p ? { status: "found", source: "demo", pharmacy: p } : { status: "not_found" };
  }
  if (!pharmacyId) return { status: "not_found" };

  let res: Response;
  try {
    res = await (opts.fetchFn ?? fetch)(pharmacyUrl(opts.gatewayUrl, pharmacyId), {
      headers: { Accept: "application/json" },
      cache: "no-store",
      signal: AbortSignal.timeout(opts.timeoutMs ?? 5000),
    });
  } catch (cause) {
    const name = (cause as { name?: unknown } | null)?.name;
    return { status: "unavailable", kind: name === "TimeoutError" || name === "AbortError" ? "timeout" : "unreachable" };
  }
  if (res.status === 404) {
    // The route answers a JSON error for an unknown pharmacy. A plain-text 404 is
    // the gateway's catch-all: the route is not mounted yet.
    const json = (res.headers.get("content-type") ?? "").includes("application/json");
    return json ? { status: "not_found" } : { status: "unavailable", kind: "not_available" };
  }
  // The route is public in the spec; 401/403 means the gateway does not serve it to patients yet.
  if (res.status === 401 || res.status === 403) return { status: "unavailable", kind: "not_available" };
  if (!res.ok) return { status: "unavailable", kind: errorForStatus(res.status).kind };

  let body: Record<string, unknown> | null;
  try {
    body = (await res.json()) as Record<string, unknown> | null;
  } catch {
    return { status: "unavailable", kind: "server" };
  }
  const pharmacy = asPharmacyDetails(body);
  if (!pharmacy) return { status: "unavailable", kind: "server" };
  if (body?.status !== undefined && body.status !== "VERIFICATION_STATUS_VERIFIED") return { status: "not_found" };
  return { status: "found", source: "gateway", pharmacy };
}

function asPharmacyDetails(body: Record<string, unknown> | null): PharmacyDetails | undefined {
  if (typeof body?.pharmacyId !== "string" || typeof body.name !== "string" || !body.name) return undefined;
  const out: PharmacyDetails = { pharmacyId: body.pharmacyId, name: body.name };
  for (const key of ["address", "dzongkhag", "phone"] as const) {
    const value = body[key];
    if (typeof value === "string" && value) out[key] = value;
  }
  const loc = body.location as Partial<GeoPoint> | undefined;
  if (typeof loc?.latitude === "number" && typeof loc.longitude === "number") {
    out.location = { latitude: loc.latitude, longitude: loc.longitude };
  }
  return out;
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

export function mockSuggest(prefix: string, limit = 8): Suggestion[] {
  const q = prefix.trim().toLowerCase();
  if (!q) return [];
  return MOCK_PRODUCTS.filter((p) => p.brandName.toLowerCase().includes(q) || p.genericName.toLowerCase().includes(q))
    .slice(0, limit)
    .map((p) => ({ productId: p.productId, displayName: `${p.brandName} (${p.genericName})` }));
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
