/** "Nu. 45.00" from chhertum (100 chhertum = Nu. 1). Missing or 0 means no price. */
export function formatPrice(chhertum: string | number | undefined): string {
  const value = typeof chhertum === "string" ? Number(chhertum) : (chhertum ?? 0);
  if (!Number.isFinite(value) || value <= 0) return "Price not listed";
  const ngultrum = Math.floor(value / 100);
  const rest = Math.round(value % 100);
  return `Nu. ${ngultrum.toLocaleString("en-IN")}.${String(rest).padStart(2, "0")}`;
}

/** "460 m" below 1 km, "2.3 km" above. */
export function formatDistance(metres: number | undefined): string {
  const m = Math.max(0, Math.round(metres ?? 0));
  if (m < 1000) return `${m} m`;
  return `${(m / 1000).toFixed(1)} km`;
}

/**
 * How much to trust the stock figure, from the freshness score (0..1).
 * Wording, not numbers: patients should not have to interpret 0.73.
 */
export function freshnessLabel(score: number | undefined): { text: string; level: "high" | "medium" | "low" } {
  const s = score ?? 0;
  if (s >= 0.75) return { text: "Stock confirmed recently", level: "high" };
  if (s >= 0.4) return { text: "Stock confirmed a while ago", level: "medium" };
  return { text: "Stock not confirmed lately – call ahead", level: "low" };
}

/** "1 unit" / "12 units". */
export function formatQuantity(n: number | undefined): string {
  const q = n ?? 0;
  return `${q} ${q === 1 ? "unit" : "units"}`;
}
