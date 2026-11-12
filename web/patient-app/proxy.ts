import type { NextRequest } from "next/server";

import { refreshInProxy } from "@/lib/session-proxy";

// Refreshes the patient's tokens shortly before the access token expires
// (lib/session-proxy.ts). Runs only when the session cookie is present, and
// not for static files.
export function proxy(request: NextRequest) {
  return refreshInProxy(request);
}

export const config = {
  matcher: [
    {
      source: "/((?!_next/static|_next/image|icon.svg|favicon.ico).*)",
      has: [{ type: "cookie", key: "swiftmed_session" }],
    },
  ],
};
