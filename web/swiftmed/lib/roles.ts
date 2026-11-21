// Who may use which part of SwiftMed (SCRUM-139). One app serves patients at
// "/", pharmacy staff at /pharmacy-portal and SwiftMed's administrators at
// /admin (SCRUM-142); the session says which one a signed-in account belongs
// to. The gateway enforces the same rules on every call; these only decide what
// the app shows and where it sends people.
import type { Session } from "./types";

/** Where the pharmacy portal lives in the app. */
export const PORTAL_PATH = "/pharmacy-portal";
/** Where administrators review pharmacies (SCRUM-142). Not linked anywhere public. */
export const ADMIN_PATH = "/admin";

/** The login page for each audience: the same page and the same sign-in (SCRUM-140). */
export const PATIENT_LOGIN = "/login";
export const STAFF_LOGIN = `/login?next=${encodeURIComponent(PORTAL_PATH)}`;
/** Sign-up for each audience (SCRUM-142). A pharmacy signs up with its details and waits for approval. */
export const PATIENT_SIGNUP = "/register";
export const PHARMACY_SIGNUP = "/register/pharmacy";

export const PATIENT_ROLE = "ROLE_PATIENT";
export const STAFF_ROLE = "ROLE_PHARMACY_STAFF";
export const ADMIN_ROLE = "ROLE_ADMIN";

type Who = Pick<Session, "role" | "pharmacyId"> | undefined;

/** Staff of a pharmacy (or an admin linked to one): they use the portal, not the patient pages. */
export function isPharmacyStaff(session: Who): boolean {
  if (!session?.pharmacyId) return false;
  return session.role === STAFF_ROLE || session.role === ADMIN_ROLE;
}

/**
 * A pharmacy's account, with or without its pharmacy yet: the portal is theirs.
 * (Without a pharmacy, the portal asks for the pharmacy's details.)
 */
export function isStaffAccount(session: Who): boolean {
  return session?.role === STAFF_ROLE || isPharmacyStaff(session);
}

/** A SwiftMed administrator not linked to a pharmacy: they use /admin. */
export function isAdmin(session: Who): boolean {
  return session?.role === ADMIN_ROLE && !session.pharmacyId;
}

/** Auth's names for the roles in the token (services/auth domain.Role), as the API names them. */
const CLAIM_ROLES: Record<string, string> = { PATIENT: PATIENT_ROLE, PHARMACIST: STAFF_ROLE, ADMIN: ADMIN_ROLE };

/**
 * The role and pharmacy in an access token. Read only, not verified: the
 * gateway verifies every call. Used after a refresh, because registering a
 * pharmacy turns a patient account into a staff account (SCRUM-141).
 */
export function claimsFromToken(accessToken: string): { role?: string; pharmacyId?: string } {
  const payload = accessToken.split(".")[1];
  if (!payload) return {};
  try {
    const claims = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as { role?: unknown; pharmacy_id?: unknown };
    const out: { role?: string; pharmacyId?: string } = {};
    if (typeof claims.role === "string") {
      const role = CLAIM_ROLES[claims.role] ?? (claims.role.startsWith("ROLE_") ? claims.role : undefined);
      if (role) out.role = role;
    }
    if (typeof claims.pharmacy_id === "string" && claims.pharmacy_id) out.pharmacyId = claims.pharmacy_id;
    return out;
  } catch {
    return {};
  }
}

/** The pharmacy_id claim of an access token, or undefined. */
export function pharmacyIdFromToken(accessToken: string): string | undefined {
  return claimsFromToken(accessToken).pharmacyId;
}

const under = (base: string) => (path: string) =>
  path === base || path.startsWith(`${base}/`) || path.startsWith(`${base}?`);

/** True for the portal and its pages (a same-site path, without the query). */
export const isPortalPath = under(PORTAL_PATH);
/** True for the admin pages. */
export const isAdminPath = under(ADMIN_PATH);

/** Where to go after signing in: admins to /admin, staff to the portal, patients to `next` (outside both). */
export function homeAfterSignIn(session: Pick<Session, "role" | "pharmacyId">, next: string): string {
  if (isAdmin(session)) return isAdminPath(next) ? next : ADMIN_PATH;
  if (isStaffAccount(session)) return isPortalPath(next) ? next : PORTAL_PATH;
  return isPortalPath(next) || isAdminPath(next) ? "/" : next;
}
