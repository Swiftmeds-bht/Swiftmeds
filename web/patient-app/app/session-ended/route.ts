import { NextResponse, type NextRequest } from "next/server";

import { SESSION_COOKIE, safeNext } from "@/lib/session";

// Pages cannot change cookies while rendering. When the gateway rejects the
// access token, a page redirects here: the stale cookie is removed and the
// patient signs in again, then returns to `next`.
export function GET(request: NextRequest) {
  const next = safeNext(request.nextUrl.searchParams.get("next"), "/holds");
  const target = new URL(`/login?next=${encodeURIComponent(next)}&expired=1`, request.url);
  const response = NextResponse.redirect(target, 303);
  response.cookies.delete(SESSION_COOKIE);
  return response;
}
