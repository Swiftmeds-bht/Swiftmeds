// Demo data used until the gateway exposes search (SCRUM-49). Taken from
// deploy/seed/*.csv so the demo matches the seeded backend. Not real
// pharmacies, prices or stock.
import type { PharmacySummary } from "./types";

export interface MockProduct {
  productId: string;
  brandName: string;
  genericName: string;
  moleculeId: string;
  strength: string;
}

export interface MockStock {
  pharmacyId: string;
  productId: string;
  quantity: number;
  priceChhertum: number;
  /** Freshness score 0..1 for the demo. */
  freshness: number;
}

const ph = (n: number) => `40000000-0000-4000-8000-0000000000${String(n).padStart(2, "0")}`;
const pr = (n: number) => `10000000-0000-4000-8000-0000000000${String(n).padStart(2, "0")}`;
const mol = (n: number) => `20000000-0000-4000-8000-0000000000${String(n).padStart(2, "0")}`;

export const MOCK_PRODUCTS: MockProduct[] = [
  { productId: pr(1), brandName: "Demo Paracetamol 500", genericName: "Paracetamol", moleculeId: mol(1), strength: "500mg" },
  { productId: pr(2), brandName: "Demo Paracetamol Syrup", genericName: "Paracetamol", moleculeId: mol(1), strength: "250mg/5ml" },
  { productId: pr(3), brandName: "Demo Ibuprofen 400", genericName: "Ibuprofen", moleculeId: mol(2), strength: "400mg" },
  { productId: pr(4), brandName: "Demo Amoxicillin 250", genericName: "Amoxicillin", moleculeId: mol(3), strength: "250mg" },
  { productId: pr(5), brandName: "Demo Amoxicillin Suspension", genericName: "Amoxicillin", moleculeId: mol(3), strength: "125mg/5ml" },
  { productId: pr(6), brandName: "Demo Cetirizine 10", genericName: "Cetirizine", moleculeId: mol(4), strength: "10mg" },
  { productId: pr(7), brandName: "Demo Omeprazole 20", genericName: "Omeprazole", moleculeId: mol(5), strength: "20mg" },
  { productId: pr(8), brandName: "Demo Metformin 500", genericName: "Metformin", moleculeId: mol(6), strength: "500mg" },
  { productId: pr(9), brandName: "Demo Amlodipine 5", genericName: "Amlodipine", moleculeId: mol(7), strength: "5mg" },
  { productId: pr(10), brandName: "Demo Losartan 50", genericName: "Losartan", moleculeId: mol(8), strength: "50mg" },
  { productId: pr(11), brandName: "Demo Aspirin 300", genericName: "Aspirin", moleculeId: mol(9), strength: "300mg" },
];

const town = (n: number) => (n <= 5 ? "Thimphu" : n <= 10 ? "Paro" : "Phuentsholing");
const COORDS: Record<number, [number, number]> = {
  1: [27.4728, 89.639], 2: [27.4764, 89.6367], 3: [27.4785, 89.6383], 4: [27.4742, 89.6435], 5: [27.4695, 89.6359],
  6: [27.4308, 89.413], 7: [27.432, 89.414], 8: [27.429, 89.412], 9: [27.428, 89.415], 10: [27.4315, 89.4105],
  11: [26.8613, 89.384], 12: [26.863, 89.385], 13: [26.8595, 89.382], 14: [26.8625, 89.388], 15: [26.858, 89.386],
};

export const MOCK_PHARMACIES: PharmacySummary[] = Object.entries(COORDS).map(([k, [latitude, longitude]]) => {
  const n = Number(k);
  const idx = String(n <= 5 ? n : n <= 10 ? n - 5 : n - 10).padStart(2, "0");
  return {
    pharmacyId: ph(n),
    name: `Demo Pharmacy ${town(n)} ${idx}`,
    address: `${town(n)} central demo area ${idx}`,
    dzongkhag: n > 10 ? "Chukha" : town(n),
    location: { latitude, longitude },
  };
});

// Quantities follow deploy/seed/stock.csv; prices and freshness are demo values.
export const MOCK_STOCK: MockStock[] = [
  { pharmacyId: ph(1), productId: pr(1), quantity: 0, priceChhertum: 1500, freshness: 0.9 },
  { pharmacyId: ph(1), productId: pr(2), quantity: 1, priceChhertum: 6500, freshness: 0.8 },
  { pharmacyId: ph(2), productId: pr(3), quantity: 24, priceChhertum: 3000, freshness: 0.95 },
  { pharmacyId: ph(2), productId: pr(4), quantity: 1, priceChhertum: 9000, freshness: 0.5 },
  { pharmacyId: ph(3), productId: pr(5), quantity: 0, priceChhertum: 11000, freshness: 0.6 },
  { pharmacyId: ph(3), productId: pr(6), quantity: 12, priceChhertum: 2500, freshness: 0.85 },
  { pharmacyId: ph(4), productId: pr(7), quantity: 1, priceChhertum: 4500, freshness: 0.3 },
  { pharmacyId: ph(4), productId: pr(8), quantity: 31, priceChhertum: 3500, freshness: 0.9 },
  { pharmacyId: ph(5), productId: pr(9), quantity: 6, priceChhertum: 4000, freshness: 0.7 },
  { pharmacyId: ph(5), productId: pr(10), quantity: 0, priceChhertum: 6000, freshness: 0.9 },
  { pharmacyId: ph(6), productId: pr(11), quantity: 1, priceChhertum: 1800, freshness: 0.65 },
  { pharmacyId: ph(3), productId: pr(1), quantity: 18, priceChhertum: 1400, freshness: 0.8 },
  { pharmacyId: ph(5), productId: pr(1), quantity: 4, priceChhertum: 1400, freshness: 0.45 },
  { pharmacyId: ph(7), productId: pr(1), quantity: 9, priceChhertum: 1600, freshness: 0.9 },
  { pharmacyId: ph(12), productId: pr(3), quantity: 15, priceChhertum: 2800, freshness: 0.75 },
];
