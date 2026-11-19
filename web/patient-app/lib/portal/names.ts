// Medicine and pharmacy names for the dashboard (SCRUM-138), from the gateway's
// public lookups getProduct and getPharmacy (docs/api/openapi.yaml). Staff see
// names, never raw IDs: when a lookup fails, a plain fallback is shown instead.
// Server-side only. Names change rarely, so answers are cached for 5 minutes.
import { cache } from "react";

import { GATEWAY_URL } from "./api";

export interface MedicineName {
  /** "Demo Paracetamol 500" */
  name: string;
  /** "500mg tablet · Paracetamol" */
  detail?: string;
  /** False when the Catalog did not know the ID (or could not be reached). */
  known: boolean;
}

const FORMS: Record<string, string> = {
  DOSAGE_FORM_TABLET: "tablet",
  DOSAGE_FORM_CAPSULE: "capsule",
  DOSAGE_FORM_SYRUP: "syrup",
  DOSAGE_FORM_SUSPENSION: "suspension",
  DOSAGE_FORM_INJECTION: "injection",
  DOSAGE_FORM_CREAM: "cream",
  DOSAGE_FORM_OINTMENT: "ointment",
  DOSAGE_FORM_DROPS: "drops",
  DOSAGE_FORM_INHALER: "inhaler",
};

export const UNKNOWN_MEDICINE: MedicineName = { name: "Unknown medicine", known: false };

interface ProductBody {
  brandName?: unknown;
  genericName?: unknown;
  strength?: unknown;
  dosageForm?: unknown;
}

const text = (v: unknown) => (typeof v === "string" ? v.trim() : "");

/** A name and a detail line from a getProduct answer. */
export function medicineName(product: ProductBody | null | undefined): MedicineName {
  const brand = text(product?.brandName);
  const generic = text(product?.genericName);
  if (!brand && !generic) return UNKNOWN_MEDICINE;
  const form = FORMS[text(product?.dosageForm)] ?? "";
  const dose = [text(product?.strength), form].filter(Boolean).join(" ");
  // The generic name helps only when it differs from the brand.
  const parts = [dose, brand && generic && generic.toLowerCase() !== brand.toLowerCase() ? generic : ""].filter(Boolean);
  return { name: brand || generic, detail: parts.join(" · ") || undefined, known: true };
}

type FetchFn = typeof fetch;

async function getJSON(path: string, fetchFn: FetchFn): Promise<unknown> {
  try {
    const res = await fetchFn(new URL(path, `${GATEWAY_URL}/`), {
      headers: { Accept: "application/json" },
      // Next.js keeps the answer for 5 minutes, across requests.
      next: { revalidate: 300 },
      signal: AbortSignal.timeout(3_000),
    } as RequestInit);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null; // a missing name must never break the page
  }
}

export async function fetchMedicineName(productId: string, fetchFn: FetchFn = fetch): Promise<MedicineName> {
  if (!productId) return UNKNOWN_MEDICINE;
  return medicineName((await getJSON(`/api/v1/products/${encodeURIComponent(productId)}`, fetchFn)) as ProductBody | null);
}

export async function fetchPharmacyName(pharmacyId: string, fetchFn: FetchFn = fetch): Promise<string | undefined> {
  if (!pharmacyId) return undefined;
  const body = (await getJSON(`/api/v1/pharmacies/${encodeURIComponent(pharmacyId)}`, fetchFn)) as { name?: unknown } | null;
  return text(body?.name) || undefined;
}

// Asked once per request even when the header and the page both need a name.
const medicineOnce = cache((productId: string) => fetchMedicineName(productId));
export const pharmacyNameOnce = cache((pharmacyId: string) => fetchPharmacyName(pharmacyId));

/** Names for many medicines at once, keyed by product ID. */
export async function lookupMedicines(productIds: (string | undefined)[]): Promise<Map<string, MedicineName>> {
  const ids = [...new Set(productIds.filter((id): id is string => Boolean(id)))];
  const names = await Promise.all(ids.map((id) => medicineOnce(id)));
  return new Map(ids.map((id, i) => [id, names[i]]));
}
