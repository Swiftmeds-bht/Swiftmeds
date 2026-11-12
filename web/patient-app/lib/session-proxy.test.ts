// @vitest-environment node
import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { forgetRefreshes } from "./refresh";
import { REFRESH_SESSION_SECONDS, SESSION_COOKIE, decodeSession, encodeSession } from "./session";
import { refreshInProxy } from "./session-proxy";
import type { Session } from "./types";

const gatewayUrl = "http://gateway:8080";
const now = Date.parse("2026-11-05T09:14:30Z");
const expiring: Session = {
  userId: "u-1",
  email: "tashi@example.bt",
  accessToken: "old-access",
  expiresAt: "2026-11-05T09:15:00Z",
  refreshToken: "old-refresh",
};
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

function requestWith(session: Session | undefined) {
  const headers = new Headers();
  if (session) headers.set("cookie", `${SESSION_COOKIE}=${encodeSession(session)}; other=1`);
  return new NextRequest("http://app.local/holds", { headers });
}

/** The Cookie header Next.js forwards to the page after the proxy. */
const forwardedCookie = (res: Response) => res.headers.get("x-middleware-request-cookie") ?? "";

afterEach(() => forgetRefreshes());

describe("refreshInProxy", () => {
  it("refreshes an access token about to expire, for this request and the browser", async () => {
    const fetchFn = vi.fn(async () =>
      json(200, { tokens: { accessToken: "new-access", refreshToken: "new-refresh", accessTokenExpiresAt: "2026-11-05T09:30:00Z" } }),
    );
    const res = await refreshInProxy(requestWith(expiring), { gatewayUrl, fetchFn, now: () => now });

    expect(fetchFn).toHaveBeenCalledTimes(1);
    const set = res.cookies.get(SESSION_COOKIE);
    expect(set?.httpOnly).toBe(true);
    expect(set?.maxAge).toBe(REFRESH_SESSION_SECONDS);
    expect(decodeSession(set?.value, now)).toMatchObject({ accessToken: "new-access", refreshToken: "new-refresh" });
    // The page rendering this same request already sees the new token.
    const forwarded = forwardedCookie(res);
    expect(forwarded).toContain(`${SESSION_COOKIE}=${set?.value}`);
    expect(forwarded).toContain("other=1");
  });

  it("also refreshes a token that has already expired", async () => {
    const fetchFn = vi.fn(async () => json(200, { tokens: { accessToken: "new-access", refreshToken: "new-refresh" } }));
    const res = await refreshInProxy(requestWith(expiring), { gatewayUrl, fetchFn, now: () => now + 10 * 60_000 });
    expect(decodeSession(res.cookies.get(SESSION_COOKIE)?.value, now + 10 * 60_000)?.accessToken).toBe("new-access");
  });

  it("signs the patient out cleanly when the refresh is refused", async () => {
    const fetchFn = vi.fn(async () => json(401, { message: "refresh token already used" }));
    const res = await refreshInProxy(requestWith(expiring), { gatewayUrl, fetchFn, now: () => now });

    // The browser is told to drop the cookie...
    const set = res.cookies.get(SESSION_COOKIE);
    expect(set?.value).toBe("");
    expect(res.headers.get("set-cookie")).toMatch(/swiftmed_session=;.*(Max-Age=0|Expires=Thu, 01 Jan 1970)/i);
    // ...and the page does not see a session, so it sends the patient to sign in.
    expect(forwardedCookie(res)).not.toContain(SESSION_COOKIE);
    expect(forwardedCookie(res)).toContain("other=1");
  });

  it("keeps the session when the gateway is down, so the next request can try again", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const fetchFn = vi.fn(async () => {
      throw new TypeError("fetch failed");
    });
    const res = await refreshInProxy(requestWith(expiring), { gatewayUrl, fetchFn, now: () => now });
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(res.headers.get("x-middleware-override-headers")).toBeNull();
    warn.mockRestore();
  });

  it("does nothing while the token is fresh, without a refresh token, or in demo mode", async () => {
    const fetchFn = vi.fn();
    const early = now - 10 * 60_000;
    for (const [session, url, at] of [
      [expiring, gatewayUrl, early],
      [{ ...expiring, refreshToken: undefined }, gatewayUrl, now],
      [expiring, "", now],
      [undefined, gatewayUrl, now],
    ] as const) {
      const res = await refreshInProxy(requestWith(session as Session | undefined), { gatewayUrl: url, fetchFn, now: () => at });
      expect(res.headers.get("set-cookie")).toBeNull();
    }
    expect(fetchFn).not.toHaveBeenCalled();
  });
});
