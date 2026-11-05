// Wording for holds: what each status means to a patient, and how long is left.
import type { HoldErrorKind } from "./holds-core";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "./mock-data";
import type { Hold, HoldStatus } from "./types";

export type Tone = "high" | "medium" | "low" | "neutral";

export const STATUS_INFO: Record<HoldStatus, { label: string; tone: Tone; final: boolean }> = {
  HOLD_STATUS_PENDING: { label: "Checking stock", tone: "medium", final: false },
  HOLD_STATUS_ACTIVE: { label: "Ready to collect", tone: "high", final: false },
  HOLD_STATUS_COLLECTED: { label: "Collected", tone: "neutral", final: true },
  HOLD_STATUS_EXPIRED: { label: "Expired", tone: "low", final: true },
  HOLD_STATUS_CANCELLED: { label: "Cancelled", tone: "neutral", final: true },
  HOLD_STATUS_REJECTED: { label: "Not available", tone: "low", final: true },
};

export function statusInfo(status: string) {
  return STATUS_INFO[status as HoldStatus] ?? { label: "Unknown", tone: "neutral" as Tone, final: true };
}

/** "29:41" under an hour, "1 h 05 min" above, "0:00" once it is over. */
export function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  if (hours > 0) return `${hours} h ${String(minutes).padStart(2, "0")} min`;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

/** "14:32" in Bhutan time, where the patients are. */
export function formatClock(iso: string | undefined): string {
  if (!iso || Number.isNaN(Date.parse(iso))) return "";
  return new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: "Asia/Thimphu" }).format(new Date(iso));
}

const ERROR_MESSAGES: Record<HoldErrorKind, string> = {
  invalid_credentials: "That email and password do not match an account.",
  email_taken: "An account with this email already exists. Sign in instead.",
  unauthorized: "Please sign in again.",
  not_found: "We could not find that. It may belong to another account.",
  not_active: "This hold is no longer active, so it cannot be changed.",
  bad_request: "Please check what you entered and try again.",
  not_available: "This is not available yet. Please try again later.",
  unreachable: "We could not reach SwiftMed. Check your connection and try again.",
  timeout: "SwiftMed took too long to answer. Please try again.",
  server: "Something went wrong on our side. Please try again in a moment.",
};

/** A sentence for the patient. `detail` is the backend's own message, used for bad input. */
export function errorMessage(kind: HoldErrorKind, detail?: string): string {
  if (kind === "bad_request" && detail) return detail;
  return ERROR_MESSAGES[kind];
}

/** Names to show for a hold. The hold only carries IDs; the demo data knows the names. */
export function holdNames(hold: Pick<Hold, "productId" | "pharmacyId">): { medicine: string; pharmacy: string; address?: string } {
  const product = MOCK_PRODUCTS.find((p) => p.productId === hold.productId);
  const pharmacy = MOCK_PHARMACIES.find((p) => p.pharmacyId === hold.pharmacyId);
  return {
    medicine: product ? `${product.brandName} ${product.strength}` : "Medicine",
    pharmacy: pharmacy?.name ?? "Pharmacy",
    address: pharmacy?.address,
  };
}
