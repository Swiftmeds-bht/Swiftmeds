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
  return { text: "Stock not confirmed lately. Call ahead", level: "low" };
}

const RELATIVE = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
// Each unit and how many of it make the next one: 60 minutes, 24 hours, 7 days.
const STEPS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["minute", 60],
  ["hour", 24],
  ["day", 7],
];

/** "just now", "5 minutes ago", "yesterday", or a date after four weeks. */
export function formatRelativeTime(iso: string, now = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  let value = Math.round((then - now) / 1000);
  if (Math.abs(value) < 45) return "just now";
  value = Math.round(value / 60);
  for (const [unit, next] of STEPS) {
    if (Math.abs(value) < next) return RELATIVE.format(value, unit);
    value = Math.round(value / next);
  }
  if (Math.abs(value) <= 4) return RELATIVE.format(value, "week");
  return new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", timeZone: "Asia/Thimphu" }).format(then);
}

/** "1 unit" / "12 units". */
export function formatQuantity(n: number | undefined): string {
  const q = n ?? 0;
  return `${q} ${q === 1 ? "unit" : "units"}`;
}
