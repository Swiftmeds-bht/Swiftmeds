// Reads and writes the session cookie. Server components and server actions only.
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { SESSION_COOKIE, decodeSession, encodeSession, sessionMaxAge } from "./session";
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
  (await cookies()).set(SESSION_COOKIE, encodeSession(session), {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge: sessionMaxAge(session),
  });
}

export async function clearSession(): Promise<void> {
  (await cookies()).delete(SESSION_COOKIE);
}
