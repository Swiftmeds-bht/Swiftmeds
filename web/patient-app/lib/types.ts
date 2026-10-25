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

export interface SearchParams {
  query: string;
  location: GeoPoint;
  radiusM: number;
}

export interface PharmacySummary {
  pharmacyId: string;
  name: string;
  address: string;
  dzongkhag: string;
  location: GeoPoint;
}
