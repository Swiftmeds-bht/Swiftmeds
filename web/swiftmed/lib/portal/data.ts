// Data the portal's header and pages share, fetched once per request (SCRUM-138,
// moved into the one app by SCRUM-139). React's cache() keeps the first answer
// for the rest of the request, so the bell and the Holds page do not ask the
// gateway twice. Server-side only.
import { cache } from "react";

import { getSession } from "../auth";
import { isPharmacyStaff } from "../roles";

import { listActiveHolds, type Hold, type PortalSession } from "./api";
import { timeLeft } from "./format";
import { lookupMedicines, pharmacyNameOnce, UNKNOWN_MEDICINE, type MedicineName } from "./names";

/** The signed-in staff member's portal session, or undefined (signed out, or not staff). Once per request. */
export const portalSession = cache(async (): Promise<PortalSession | undefined> => {
  const session = await getSession();
  if (!session || !isPharmacyStaff(session) || !session.pharmacyId) return undefined;
  return { pharmacyId: session.pharmacyId, accessToken: session.accessToken, email: session.email };
});

/** Active holds at the session's pharmacy, fetched once per request. */
export const activeHolds = cache((session: PortalSession): Promise<Hold[]> => listActiveHolds(session));

/** The pharmacy's name for headings, or a neutral fallback. */
export async function pharmacyLabel(session: PortalSession): Promise<string> {
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
