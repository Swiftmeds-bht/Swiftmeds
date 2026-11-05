// The signed-in patient is kept in one httpOnly cookie, so page scripts cannot
// read the access token. This file only encodes and decodes the value; reading
// and writing the cookie is in lib/auth.ts (server only).
import type { Session } from "./types";

export const SESSION_COOKIE = "swiftmed_session";
/** Used when the gateway does not say when the access token expires. */
export const DEFAULT_SESSION_SECONDS = 15 * 60;

export function encodeSession(session: Session): string {
  return Buffer.from(JSON.stringify(session), "utf8").toString("base64url");
}

/** The session in a cookie value, or undefined if it is missing, malformed or expired. */
export function decodeSession(value: string | undefined, now = Date.now()): Session | undefined {
  if (!value) return undefined;
  let parsed: Partial<Session>;
  try {
    parsed = JSON.parse(Buffer.from(value, "base64url").toString("utf8")) as Partial<Session>;
  } catch {
    return undefined;
  }
  const { userId, email, accessToken, expiresAt } = parsed;
  if (typeof userId !== "string" || typeof email !== "string" || typeof accessToken !== "string") return undefined;
  if (!userId || !accessToken) return undefined;
  if (expiresAt !== undefined && (typeof expiresAt !== "string" || !(Date.parse(expiresAt) > now))) return undefined;
  return expiresAt === undefined ? { userId, email, accessToken } : { userId, email, accessToken, expiresAt };
}

/** Cookie lifetime: until the access token expires, or the default. */
export function sessionMaxAge(session: Session, now = Date.now()): number {
  if (!session.expiresAt) return DEFAULT_SESSION_SECONDS;
  return Math.max(0, Math.floor((Date.parse(session.expiresAt) - now) / 1000));
}

/** Only same-site paths, so a crafted ?next= link cannot send a patient elsewhere after signing in. */
export function safeNext(next: string | undefined | null, fallback = "/holds"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}
