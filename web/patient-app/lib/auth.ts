// Reads and writes the session cookie. Server components and server actions only.
// Server components can only read it; proxy.ts refreshes the tokens before they
// render, and server actions save new ones (withFreshToken).
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import { retryAfterRefresh, revokeRefreshToken } from "./refresh";
import { SESSION_COOKIE, decodeSession, encodeSession, parseSession, sessionCookieOptions } from "./session";
import type { Session } from "./types";

export async function getSession(): Promise<Session | undefined> {
  return decodeSession((await cookies()).get(SESSION_COOKIE)?.value);
}

/** The session, or a redirect to sign in that comes back to `returnTo` afterwards. */
export async function requireSession(returnTo: string): Promise<Session> {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(returnTo)}`);
  return session;
}

export async function saveSession(session: Session): Promise<void> {
  (await cookies()).set(SESSION_COOKIE, encodeSession(session), sessionCookieOptions(session));
}

/** Signs out: forgets the cookie and, with a gateway, revokes the refresh token (best effort). */
export async function clearSession(gatewayUrl = configuredGatewayUrl()): Promise<void> {
  const jar = await cookies();
  const refreshToken = parseSession(jar.get(SESSION_COOKIE)?.value)?.refreshToken;
  jar.delete(SESSION_COOKIE);
  if (refreshToken && gatewayUrl) await revokeRefreshToken(refreshToken, { gatewayUrl });
}

/**
 * Runs a gateway call with the session. If the gateway answers 401 (for
 * example the token was revoked, or the clocks disagree), it refreshes the
 * tokens once, saves them and tries again. Server actions only, since it sets
 * the cookie. If the refresh is refused, the original error is thrown and the
 * caller signs the patient out.
 */
export function withFreshToken<T>(session: Session, run: (session: Session) => Promise<T>): Promise<T> {
  return retryAfterRefresh(session, run, { gatewayUrl: configuredGatewayUrl(), save: saveSession });
}
