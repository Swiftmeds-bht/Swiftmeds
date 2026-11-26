// Prices on the portal's Stock page (SCRUM-154). Staff type ngultrum
// ("45" or "45.50"); the API takes chhertum (100 chhertum = Nu. 1).

/** Nu. 1,000,000 a pack, the gateway's limit: catches a typo with extra zeros. */
export const MAX_PRICE_CHHERTUM = 100_000_000;

/** "45.5" -> 4550 chhertum, or an error to show next to the form. */
export function parsePriceNu(text: string): { chhertum: number } | { error: string } {
  const value = text.trim().replace(/^nu\.?\s*/i, "").replace(/,/g, "");
  if (!/^\d+(\.\d{1,2})?$/.test(value)) {
    return { error: "Enter the price in ngultrum, for example 45 or 45.50." };
  }
  const [whole, fraction = ""] = value.split(".");
  const chhertum = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  if (!Number.isSafeInteger(chhertum) || chhertum <= 0) return { error: "The price must be more than Nu. 0." };
  if (chhertum > MAX_PRICE_CHHERTUM) return { error: "The price can be at most Nu. 1,000,000." };
  return { chhertum };
}

/** 4550 -> "45.50", for the price field's starting value. */
export function chhertumToNu(chhertum: number): string {
  return (chhertum / 100).toFixed(2);
}
