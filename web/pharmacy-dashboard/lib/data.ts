// Data the header and the pages share, fetched once per request (SCRUM-138):
// React's cache() keeps the first answer for the rest of the request, so the
// bell and the Holds page do not ask the gateway twice. Server-side only.
import { cache } from "react";

import { listActiveHolds, type Hold } from "./api";
import { timeLeft } from "./format";
import { lookupMedicines, pharmacyNameOnce, UNKNOWN_MEDICINE, type MedicineName } from "./names";
import { getSession, type DashboardSession } from "./session";

/** The signed-in staff session, read once per request. */
export const currentSession = cache(getSession);

/** Active holds at the session's pharmacy, fetched once per request. */
export const activeHolds = cache((session: DashboardSession): Promise<Hold[]> => listActiveHolds(session));

/** The pharmacy's name for headings, or a neutral fallback. */
export async function pharmacyLabel(session: DashboardSession): Promise<string> {
  return (await pharmacyNameOnce(session.pharmacyId)) ?? "Your pharmacy";
}

export interface NamedHold extends Hold {
  medicine: MedicineName;
  timeLeft: string;
}

/** Holds with their medicine names, soonest to expire first. */
export async function nameHolds(holds: Hold[], now = Date.now()): Promise<NamedHold[]> {
  const names = await lookupMedicines(holds.map((h) => h.productId));
  return holds
    .map((h) => ({ ...h, medicine: names.get(h.productId ?? "") ?? UNKNOWN_MEDICINE, timeLeft: timeLeft(h.expiresAt, now) }))
    .sort((a, b) => (Date.parse(a.expiresAt ?? "") || Infinity) - (Date.parse(b.expiresAt ?? "") || Infinity));
}
