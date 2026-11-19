// Real medicine and pharmacy names for holds, alerts and notifications
// (SCRUM-138), from the gateway's public lookups getProduct and getPharmacy
// (docs/api/openapi.yaml). Holds and alerts carry only IDs; a page gathers the
// names it needs into a NameBook in one go. In demo mode (USE_DEMO_DATA=true),
// or when a lookup fails, holdNames() falls back to the demo data.
// Server-side only. Names change rarely, so answers are cached for 5 minutes.
import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import { EMPTY_BOOK, type NameBook } from "./name-book";

export { EMPTY_BOOK, type NameBook };

export interface NameBookOptions {
  gatewayUrl?: string;
  fetchFn?: typeof fetch;
}

const text = (v: unknown) => (typeof v === "string" ? v.trim() : "");

async function getJSON(url: URL, fetchFn: typeof fetch): Promise<Record<string, unknown> | null> {
  try {
    const res = await fetchFn(url, {
      headers: { Accept: "application/json" },
      next: { revalidate: 300 },
      signal: AbortSignal.timeout(3_000),
    } as RequestInit);
    if (!res.ok) return null;
    const body: unknown = await res.json();
    return typeof body === "object" && body !== null ? (body as Record<string, unknown>) : null;
  } catch {
    return null; // a missing name falls back; it never breaks the page
  }
}

/** Looks up every medicine and pharmacy the items mention, once each. */
export async function nameBook(items: { productId?: string; pharmacyId?: string }[], options: NameBookOptions = {}): Promise<NameBook> {
  const gatewayUrl = options.gatewayUrl ?? configuredGatewayUrl();
  if (!gatewayUrl) return EMPTY_BOOK; // demo mode: the demo data knows the names
  const fetchFn = options.fetchFn ?? fetch;
  const unique = (ids: (string | undefined)[]) => [...new Set(ids.filter((id): id is string => Boolean(id)))];
  const productIds = unique(items.map((i) => i.productId));
  const pharmacyIds = unique(items.map((i) => i.pharmacyId));
  const url = (path: string, id: string) => new URL(`${path}/${encodeURIComponent(id)}`, gatewayUrl);

  const [productBodies, pharmacyBodies] = await Promise.all([
    Promise.all(productIds.map((id) => getJSON(url("/api/v1/products", id), fetchFn))),
    Promise.all(pharmacyIds.map((id) => getJSON(url("/api/v1/pharmacies", id), fetchFn))),
  ]);

  const products = new Map<string, { brandName: string; strength?: string }>();
  productIds.forEach((id, i) => {
    const brandName = text(productBodies[i]?.brandName) || text(productBodies[i]?.genericName);
    if (brandName) products.set(id, { brandName, strength: text(productBodies[i]?.strength) || undefined });
  });
  const pharmacies = new Map<string, { name: string; address?: string }>();
  pharmacyIds.forEach((id, i) => {
    const name = text(pharmacyBodies[i]?.name);
    if (name) pharmacies.set(id, { name, address: text(pharmacyBodies[i]?.address) || undefined });
  });
  return { products, pharmacies };
}
