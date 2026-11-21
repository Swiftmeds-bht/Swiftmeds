// What proxy.ts does for every request that carries the session cookie
// (SCRUM-131). Server components cannot set cookies, so the tokens are
// refreshed here, before the page, route handler or server action runs:
// - the new cookie goes into the forwarded request, so getSession() in this
//   same request already sees the new access token, and
// - into the response, so the browser keeps it.
// A refused refresh removes the cookie in both places: the patient is signed
// out, and pages that need a session send them to sign in. A failed refresh
// (gateway down) changes nothing, and the next request tries again.
import { NextResponse, type NextRequest } from "next/server";

import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import { refreshOnce, type RefreshOptions } from "./refresh";
import { SESSION_COOKIE, encodeSession, needsRefresh, parseSession, sessionCookieOptions } from "./session";

export async function refreshInProxy(request: NextRequest, options: Partial<RefreshOptions> = {}): Promise<NextResponse> {
  const gatewayUrl = options.gatewayUrl ?? configuredGatewayUrl();
  const now = options.now ?? Date.now;
  const session = parseSession(request.cookies.get(SESSION_COOKIE)?.value);
  if (!gatewayUrl || !session || !needsRefresh(session, now())) return NextResponse.next();

  const outcome = await refreshOnce(session, { ...options, gatewayUrl, now });
  if (outcome.kind === "failed") {
    console.warn(`session refresh failed (${outcome.reason}); keeping the session for now`);
    return NextResponse.next();
  }

  if (outcome.kind === "refreshed") {
    const value = encodeSession(outcome.session);
    request.cookies.set(SESSION_COOKIE, value);
    const response = NextResponse.next({ request: { headers: new Headers(request.headers) } });
    response.cookies.set(SESSION_COOKIE, value, sessionCookieOptions(outcome.session, now()));
    return response;
  }

  request.cookies.delete(SESSION_COOKIE);
  const response = NextResponse.next({ request: { headers: new Headers(request.headers) } });
  response.cookies.delete(SESSION_COOKIE);
  return response;
}
