// Pharmacy sign-up and the admin review (SCRUM-142), against the gateway
// routes of SCRUM-141 (docs/api/openapi.yaml: registerPharmacy, getPharmacy,
// listPharmacies, setPharmacyVerification). Server-side only.
import { cache } from "react";

import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import type { GeoPoint, Session } from "./types";

export type VerificationStatus =
  | "VERIFICATION_STATUS_PENDING"
  | "VERIFICATION_STATUS_VERIFIED"
  | "VERIFICATION_STATUS_REJECTED"
  | "VERIFICATION_STATUS_SUSPENDED";

export const PENDING: VerificationStatus = "VERIFICATION_STATUS_PENDING";
export const VERIFIED: VerificationStatus = "VERIFICATION_STATUS_VERIFIED";
export const REJECTED: VerificationStatus = "VERIFICATION_STATUS_REJECTED";
export const SUSPENDED: VerificationStatus = "VERIFICATION_STATUS_SUSPENDED";

/** The admin page's tabs, in order, with the words used for each status. */
export const STATUS_TABS: { status: VerificationStatus; slug: string; label: string }[] = [
  { status: PENDING, slug: "pending", label: "Waiting for review" },
  { status: VERIFIED, slug: "verified", label: "Approved" },
  { status: REJECTED, slug: "rejected", label: "Rejected" },
  { status: SUSPENDED, slug: "suspended", label: "Suspended" },
];

export function statusFromSlug(slug: string | undefined): VerificationStatus {
  return STATUS_TABS.find((t) => t.slug === slug)?.status ?? PENDING;
}

/** openapi.yaml Pharmacy. ownerUserId and licenseNumber only reach the pharmacy's own staff and admins. */
export interface Pharmacy {
  pharmacyId: string;
  ownerUserId?: string;
  name: string;
  licenseNumber?: string;
  phone?: string;
  address?: string;
  dzongkhag?: string;
  location?: GeoPoint;
  status: VerificationStatus;
  statusReason?: string;
  createdAt?: string;
  verifiedAt?: string;
}

export interface PharmacyDetails {
  name: string;
  licenseNumber: string;
  phone: string;
  address: string;
  dzongkhag: string;
  location: GeoPoint;
}

/** A refused or failed gateway call. `message` is safe to show. */
export class PharmacyApiError extends Error {
  constructor(
    message: string,
    readonly status = 0,
  ) {
    super(message);
    this.name = "PharmacyApiError";
  }
}

const TIMEOUT_MS = 6_000;

function asPharmacy(value: unknown): Pharmacy {
  const p = (value ?? {}) as Partial<Pharmacy>;
  if (typeof p.pharmacyId !== "string" || !p.pharmacyId) throw new PharmacyApiError("The gateway sent a pharmacy without an ID.");
  return { ...p, pharmacyId: p.pharmacyId, name: typeof p.name === "string" ? p.name : "", status: (p.status as VerificationStatus) ?? PENDING };
}

function friendly(status: number, body: unknown): string {
  const sent = (body as { error?: unknown } | null)?.error;
  if (status >= 400 && status < 500 && typeof sent === "string" && sent) return `${sent[0].toUpperCase()}${sent.slice(1)}.`;
  if (status === 401) return "Your session has expired. Please log in again.";
  if (status === 403) return "This account cannot do that.";
  if (status === 502) return "The change was saved, but the map could not be updated. Press the button again.";
  return "SwiftMed could not do that just now. Please try again in a moment.";
}

async function call(method: "GET" | "POST" | "PUT", path: string, options: { token?: string; body?: unknown } = {}): Promise<unknown> {
  const base = configuredGatewayUrl();
  if (!base) throw new PharmacyApiError("Pharmacy sign-up and review need the live service (the app runs on demo data).", 503);
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.token) headers.Authorization = `Bearer ${options.token}`;
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  let res: Response;
  try {
    res = await fetch(new URL(path, `${base}/`), {
      method,
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      cache: "no-store",
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch {
    throw new PharmacyApiError("SwiftMed did not answer in time. Please try again.");
  }
  const text = await res.text();
  let body: unknown = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    // keep null
  }
  if (!res.ok) throw new PharmacyApiError(friendly(res.status, body), res.status);
  return body;
}

/** registerPharmacy: the signed-in account becomes the pharmacy's staff account; the pharmacy starts pending. */
export async function registerPharmacy(session: Session, details: PharmacyDetails): Promise<Pharmacy> {
  return asPharmacy(await call("POST", "/api/v1/pharmacies", { token: session.accessToken, body: details }));
}

/** The pharmacy as everyone may see it (status included), fresh on every request. Undefined if unknown or unreachable. */
export const pharmacyOnce = cache(async (pharmacyId: string): Promise<Pharmacy | undefined> => {
  try {
    return asPharmacy(await call("GET", `/api/v1/pharmacies/${encodeURIComponent(pharmacyId)}`));
  } catch {
    return undefined;
  }
});

/** listPharmacies (admins). */
export async function listPharmacies(session: Session, status: VerificationStatus): Promise<Pharmacy[]> {
  const body = (await call("GET", `/api/v1/pharmacies?status=${status}&pageSize=100`, { token: session.accessToken })) as {
    pharmacies?: unknown[];
  } | null;
  return (body?.pharmacies ?? []).map(asPharmacy).sort((a, b) => (a.createdAt ?? "").localeCompare(b.createdAt ?? ""));
}

/** setPharmacyVerification (admins): approve, reject or suspend. */
export async function setVerification(session: Session, pharmacyId: string, status: VerificationStatus, reason: string): Promise<Pharmacy> {
  return asPharmacy(
    await call("PUT", `/api/v1/pharmacies/${encodeURIComponent(pharmacyId)}/verification`, {
      token: session.accessToken,
      body: reason ? { status, reason } : { status },
    }),
  );
}
