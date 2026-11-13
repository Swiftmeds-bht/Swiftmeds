// Shapes of the search results as JSON, mirroring proto/search/v1/search.proto
// (protobuf JSON uses camelCase, and int64 values such as money arrive as strings).

export interface GeoPoint {
  latitude: number;
  longitude: number;
}

export interface PharmacyOffer {
  pharmacyId: string;
  pharmacyName: string;
  address: string;
  location?: GeoPoint;
  distanceM: number;
  quantityAvailable: number;
  /** Chhertum (100 = Nu. 1). "0" or absent when the pharmacy has not set a price. */
  priceChhertum?: string;
  freshnessScore: number;
  trustScore: number;
  rankScore: number;
  stockConfirmedAt?: string;
}

export interface ProductResult {
  productId: string;
  brandName: string;
  genericName: string;
  strength: string;
  offers: PharmacyOffer[];
}

export interface SearchResponse {
  searchId: string;
  results: ProductResult[];
  /** Only filled when nothing matching the query is in stock nearby. */
  substitutes: ProductResult[];
}

/** One name offered while the patient types (proto/search/v1 Suggestion). */
export interface Suggestion {
  productId: string;
  /** e.g. "Napa 500mg (Paracetamol)" */
  displayName: string;
}

export interface SearchParams {
  query: string;
  location: GeoPoint;
  radiusM: number;
}

// Holds and accounts, mirroring docs/api/openapi.yaml (Hold, HoldStatus, LoginResponse).

export type HoldStatus =
  | "HOLD_STATUS_PENDING"
  | "HOLD_STATUS_ACTIVE"
  | "HOLD_STATUS_COLLECTED"
  | "HOLD_STATUS_EXPIRED"
  | "HOLD_STATUS_CANCELLED"
  | "HOLD_STATUS_REJECTED";

export interface Hold {
  holdId: string;
  userId?: string;
  pharmacyId?: string;
  productId?: string;
  quantity?: number;
  status: HoldStatus;
  /** Set once the hold is active; only the patient who made it sees it. */
  pickupCode?: string;
  createdAt?: string;
  /** Set when the hold becomes active. */
  expiresAt?: string;
  statusChangedAt?: string;
  /** Set for rejected and cancelled holds. */
  statusReason?: string;
}

export interface CreateHoldInput {
  pharmacyId: string;
  productId: string;
  quantity: number;
}

/** "Tell me when this is back in stock near me" (openapi.yaml Subscription). */
export interface WatchSubscription {
  subscriptionId: string;
  productId: string;
  location?: GeoPoint;
  radiusM?: number;
  createdAt?: string;
  /** Set once the alert fired; the subscription does not fire again. */
  notifiedAt?: string;
}

/** One back-in-stock alert from the alert stream (openapi.yaml StockAlert). */
export interface StockAlert {
  subscriptionId: string;
  productId: string;
  pharmacyId: string;
  quantityAvailable: number;
  occurredAt: string;
}

/** The signed-in patient, kept in an httpOnly cookie (lib/session.ts). */
export interface Session {
  userId: string;
  email: string;
  accessToken: string;
  /** ISO time the access token stops working, when the gateway says. */
  expiresAt?: string;
  /**
   * Exchanged for a new token pair shortly before `expiresAt` (lib/refresh.ts).
   * Server-side only, like the access token. Absent in demo mode.
   */
  refreshToken?: string;
}

export interface PharmacySummary {
  pharmacyId: string;
  name: string;
  address: string;
  dzongkhag: string;
  location: GeoPoint;
}

/** What the pharmacy page shows (openapi.yaml Pharmacy, public fields only). */
export interface PharmacyDetails {
  pharmacyId: string;
  name: string;
  address?: string;
  dzongkhag?: string;
  phone?: string;
  location?: GeoPoint;
}
