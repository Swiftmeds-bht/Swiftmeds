// The signed-in patient is kept in one httpOnly cookie, so page scripts cannot
// read the access or refresh token. This file only encodes and decodes the
// value; reading and writing the cookie is in lib/auth.ts and proxy.ts (server only).
import type { Session } from "./types";

export const SESSION_COOKIE = "swiftmed_session";
/** Used when the gateway does not say when the access token expires (the gateway's JWT_TTL default). */
export const DEFAULT_SESSION_SECONDS = 15 * 60;
/**
 * Cookie lifetime when there is a refresh token. The gateway decides how long
 * the refresh token really lasts (JWT_REFRESH_TTL, 30 days by default); the
 * cookie is kept shorter so a forgotten device is signed out within a week.
 */
export const REFRESH_SESSION_SECONDS = 7 * 24 * 60 * 60;
/** Refresh this long before the access token expires, so a request never goes out with a token about to lapse. */
export const REFRESH_MARGIN_MS = 60_000;

export function encodeSession(session: Session): string {
  return Buffer.from(JSON.stringify(session), "utf8").toString("base64url");
}

/** The session in a cookie value, expired or not; undefined if it is missing or malformed. */
export function parseSession(value: string | undefined): Session | undefined {
  if (!value) return undefined;
  let parsed: Partial<Session>;
  try {
    parsed = JSON.parse(Buffer.from(value, "base64url").toString("utf8")) as Partial<Session>;
  } catch {
    return undefined;
  }
  const { userId, email, accessToken, expiresAt, refreshToken, role, pharmacyId } = parsed ?? {};
  if (typeof userId !== "string" || typeof email !== "string" || typeof accessToken !== "string") return undefined;
  if (!userId || !accessToken) return undefined;
  if (expiresAt !== undefined && (typeof expiresAt !== "string" || Number.isNaN(Date.parse(expiresAt)))) return undefined;
  if (refreshToken !== undefined && (typeof refreshToken !== "string" || !refreshToken)) return undefined;
  const session: Session = { userId, email, accessToken };
  if (expiresAt !== undefined) session.expiresAt = expiresAt;
  if (refreshToken !== undefined) session.refreshToken = refreshToken;
  if (typeof role === "string" && role) session.role = role;
  if (typeof pharmacyId === "string" && pharmacyId) session.pharmacyId = pharmacyId;
  return session;
}

/** The session in a cookie value, or undefined if it is missing, malformed or its access token expired. */
export function decodeSession(value: string | undefined, now = Date.now()): Session | undefined {
  const session = parseSession(value);
  if (!session) return undefined;
  if (session.expiresAt !== undefined && !(Date.parse(session.expiresAt) > now)) return undefined;
  return session;
}

/** True when the session has a refresh token and its access token expires within the margin (or already has). */
export function needsRefresh(session: Session, now = Date.now(), marginMs = REFRESH_MARGIN_MS): boolean {
  if (!session.refreshToken || !session.expiresAt) return false;
  return Date.parse(session.expiresAt) - now <= marginMs;
}

/** Cookie lifetime: long enough to refresh when there is a refresh token, else until the access token expires. */
export function sessionMaxAge(session: Session, now = Date.now()): number {
  if (session.refreshToken) return REFRESH_SESSION_SECONDS;
  if (!session.expiresAt) return DEFAULT_SESSION_SECONDS;
  return Math.max(0, Math.floor((Date.parse(session.expiresAt) - now) / 1000));
}

/** The cookie attributes, shared by server actions (lib/auth.ts) and proxy.ts. */
export function sessionCookieOptions(session: Session, now = Date.now()) {
  return {
    httpOnly: true,
    sameSite: "lax" as const,
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge: sessionMaxAge(session, now),
  };
}

/** Only same-site paths, so a crafted ?next= link cannot send a patient elsewhere after signing in. */
export function safeNext(next: string | undefined | null, fallback = "/holds"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}
