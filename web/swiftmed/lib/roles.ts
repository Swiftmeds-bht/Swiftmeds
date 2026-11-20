// Who may use which part of SwiftMed (SCRUM-139). One app serves patients at
// "/" and pharmacy staff at /pharmacy-portal; the session says which one a
// signed-in account belongs to. The gateway enforces the same rules on every
// call; these only decide what the app shows and where it sends people.
import type { Session } from "./types";

/** Where the pharmacy portal lives in the app. */
export const PORTAL_PATH = "/pharmacy-portal";

/** The login page for each audience: the same page and the same sign-in (SCRUM-140). */
export const PATIENT_LOGIN = "/login";
export const STAFF_LOGIN = `/login?next=${encodeURIComponent(PORTAL_PATH)}`;

export const STAFF_ROLE = "ROLE_PHARMACY_STAFF";
export const ADMIN_ROLE = "ROLE_ADMIN";

/** Staff of a pharmacy (or an admin linked to one): they use the portal, not the patient pages. */
export function isPharmacyStaff(session: Pick<Session, "role" | "pharmacyId"> | undefined): boolean {
  if (!session?.pharmacyId) return false;
  return session.role === STAFF_ROLE || session.role === ADMIN_ROLE;
}

/** The pharmacy_id claim of an access token, or undefined. Read only, not verified: the gateway verifies every call. */
export function pharmacyIdFromToken(accessToken: string): string | undefined {
  const payload = accessToken.split(".")[1];
  if (!payload) return undefined;
  try {
    const claims = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as { pharmacy_id?: unknown };
    return typeof claims.pharmacy_id === "string" && claims.pharmacy_id ? claims.pharmacy_id : undefined;
  } catch {
    return undefined;
  }
}

/** True for the portal and its pages (a same-site path, without the query). */
export function isPortalPath(path: string): boolean {
  return path === PORTAL_PATH || path.startsWith(`${PORTAL_PATH}/`) || path.startsWith(`${PORTAL_PATH}?`);
}

/** Where to go after signing in: staff to the portal, patients to `next` (outside the portal). */
export function homeAfterSignIn(session: Pick<Session, "role" | "pharmacyId">, next: string): string {
  const inPortal = isPortalPath(next);
  if (isPharmacyStaff(session)) return inPortal ? next : PORTAL_PATH;
  return inPortal ? "/" : next;
}
