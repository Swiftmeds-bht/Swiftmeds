// Dates and times as the dashboard shows them, in Bhutan time.

/** "19 Nov 2026, 14:05", or "—" when missing or not a date. */
export function displayDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("en-GB", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Thimphu" }).format(date);
}

/** "1 h 20 min left", "12 min left", "Expired", or "" when unknown. */
export function timeLeft(expiresAt: string | undefined, now = Date.now()): string {
  const end = expiresAt ? Date.parse(expiresAt) : Number.NaN;
  if (Number.isNaN(end)) return "";
  const minutes = Math.ceil((end - now) / 60_000);
  if (minutes <= 0) return "Expired";
  if (minutes < 60) return `${minutes} min left`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest ? `${hours} h ${rest} min left` : `${hours} h left`;
}
