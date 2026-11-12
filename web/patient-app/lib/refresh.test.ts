// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

import { HoldError } from "./holds-core";
import {
  REFRESH_REUSE_MS,
  forgetRefreshes,
  refreshOnce,
  refreshSession,
  retryAfterRefresh,
  revokeRefreshToken,
  tokenFields,
} from "./refresh";
import type { Session } from "./types";

const gatewayUrl = "http://gateway:8080";
const now = Date.parse("2026-11-05T09:14:30Z");
const session: Session = {
  userId: "u-1",
  email: "tashi@example.bt",
  accessToken: "old-access",
  expiresAt: "2026-11-05T09:15:00Z",
  refreshToken: "old-refresh",
};
const newTokens = { accessToken: "new-access", refreshToken: "new-refresh", accessTokenExpiresAt: "2026-11-05T09:30:00Z" };
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

afterEach(() => forgetRefreshes());

describe("refreshSession", () => {
  it("sends the refresh token to the gateway and returns the new pair", async () => {
    const fetchFn = vi.fn(async () => json(200, { tokens: newTokens }));
    const outcome = await refreshSession(session, { gatewayUrl, fetchFn, now: () => now });

    expect(outcome).toEqual({
      kind: "refreshed",
      session: { userId: "u-1", email: "tashi@example.bt", accessToken: "new-access", refreshToken: "new-refresh", expiresAt: "2026-11-05T09:30:00Z" },
    });
    const [url, init] = fetchFn.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("http://gateway:8080/api/v1/auth/refresh");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ refreshToken: "old-refresh" });
    expect(new Headers(init.headers).get("authorization")).toBeNull();
  });

  it.each([400, 401, 403, 404])("treats HTTP %i as a refused refresh", async (status) => {
    const fetchFn = vi.fn(async () => json(status, { message: "no" }));
    expect(await refreshSession(session, { gatewayUrl, fetchFn })).toEqual({ kind: "rejected", status });
  });

  it("treats a down, slow or broken gateway as a failure to retry, not a sign-out", async () => {
    const down = vi.fn(async () => {
      throw new TypeError("fetch failed");
    });
    expect((await refreshSession(session, { gatewayUrl, fetchFn: down })).kind).toBe("failed");
    const slow = vi.fn(async () => {
      throw new DOMException("timed out", "TimeoutError");
    });
    expect(await refreshSession(session, { gatewayUrl, fetchFn: slow })).toEqual({ kind: "failed", reason: "timeout" });
    expect((await refreshSession(session, { gatewayUrl, fetchFn: vi.fn(async () => json(503, {})) })).kind).toBe("failed");
    expect((await refreshSession(session, { gatewayUrl, fetchFn: vi.fn(async () => json(200, { tokens: {} })) })).kind).toBe(
      "failed",
    );
  });

  it("drops the spent refresh token when the gateway sends no new one", async () => {
    const fetchFn = vi.fn(async () => json(200, { tokens: { accessToken: "new-access", accessTokenExpiresAt: newTokens.accessTokenExpiresAt } }));
    const outcome = await refreshSession(session, { gatewayUrl, fetchFn });
    expect(outcome.kind).toBe("refreshed");
    expect(outcome.kind === "refreshed" && "refreshToken" in outcome.session).toBe(false);
  });

  it("does not call the gateway without a refresh token", async () => {
    const fetchFn = vi.fn();
    const { refreshToken: _r, ...noRefresh } = session;
    void _r;
    expect((await refreshSession(noRefresh, { gatewayUrl, fetchFn })).kind).toBe("rejected");
    expect(fetchFn).not.toHaveBeenCalled();
  });
});

describe("tokenFields", () => {
  it("assumes the default 15 minutes when the gateway sends no expiry", () => {
    expect(tokenFields({ accessToken: "a" }, now)).toEqual({ accessToken: "a", expiresAt: new Date(now + 15 * 60_000).toISOString() });
    expect(tokenFields({ accessToken: "" }, now)).toBeUndefined();
    expect(tokenFields(undefined, now)).toBeUndefined();
  });
});

describe("refreshOnce", () => {
  it("shares one gateway call between concurrent requests with the same cookie", async () => {
    let release!: (r: Response) => void;
    const fetchFn = vi.fn(() => new Promise<Response>((resolve) => (release = resolve)));
    const a = refreshOnce(session, { gatewayUrl, fetchFn, now: () => now });
    const b = refreshOnce(session, { gatewayUrl, fetchFn, now: () => now });
    release(json(200, { tokens: newTokens }));

    const [ra, rb] = await Promise.all([a, b]);
    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(ra).toEqual(rb);
    expect(ra.kind).toBe("refreshed");
  });

  it("gives a late request with the old cookie the same new tokens for a short while", async () => {
    let t = now;
    const fetchFn = vi.fn(async () => json(200, { tokens: newTokens }));
    const first = await refreshOnce(session, { gatewayUrl, fetchFn, now: () => t });
    t += REFRESH_REUSE_MS - 1;
    expect(await refreshOnce(session, { gatewayUrl, fetchFn, now: () => t })).toEqual(first);
    expect(fetchFn).toHaveBeenCalledTimes(1);

    t += 2;
    await refreshOnce(session, { gatewayUrl, fetchFn, now: () => t });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("does not remember failures, so the next request tries again", async () => {
    const fetchFn = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(503, {}))
      .mockResolvedValueOnce(json(200, { tokens: newTokens }));
    expect((await refreshOnce(session, { gatewayUrl, fetchFn })).kind).toBe("failed");
    expect((await refreshOnce(session, { gatewayUrl, fetchFn })).kind).toBe("refreshed");
  });
});

describe("retryAfterRefresh (server actions after a 401)", () => {
  const unauthorized = () => new HoldError("unauthorized", "Not signed in, or the session ended");

  it("refreshes once, saves the new session and retries the call", async () => {
    const fetchFn = vi.fn(async () => json(200, { tokens: newTokens }));
    const save = vi.fn(async () => {});
    const run = vi.fn(async (s: Session) => {
      if (s.accessToken === "old-access") throw unauthorized();
      return `ok with ${s.accessToken}`;
    });

    expect(await retryAfterRefresh(session, run, { gatewayUrl, fetchFn, save })).toBe("ok with new-access");
    expect(run).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ accessToken: "new-access", refreshToken: "new-refresh" }));
  });

  it("throws the 401 when the refresh is refused, so the action signs the patient out", async () => {
    const fetchFn = vi.fn(async () => json(401, { message: "refresh token revoked" }));
    const save = vi.fn(async () => {});
    const err = unauthorized();
    await expect(retryAfterRefresh(session, async () => Promise.reject(err), { gatewayUrl, fetchFn, save })).rejects.toBe(err);
    expect(save).not.toHaveBeenCalled();
  });

  it("does not loop when the new token is rejected too", async () => {
    const fetchFn = vi.fn(async () => json(200, { tokens: newTokens }));
    const run = vi.fn(async () => Promise.reject(unauthorized()));
    await expect(retryAfterRefresh(session, run, { gatewayUrl, fetchFn, save: async () => {} })).rejects.toBeInstanceOf(HoldError);
    expect(run).toHaveBeenCalledTimes(2);
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it("leaves other errors alone", async () => {
    const fetchFn = vi.fn();
    const err = new HoldError("timeout", "slow");
    await expect(retryAfterRefresh(session, async () => Promise.reject(err), { gatewayUrl, fetchFn, save: async () => {} })).rejects.toBe(err);
    expect(fetchFn).not.toHaveBeenCalled();
  });
});

describe("revokeRefreshToken", () => {
  it("posts the token to logout and never throws", async () => {
    const fetchFn = vi.fn(async () => new Response(null, { status: 204 }));
    await revokeRefreshToken("r-1", { gatewayUrl, fetchFn });
    const [url, init] = fetchFn.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("http://gateway:8080/api/v1/auth/logout");
    expect(JSON.parse(String(init.body))).toEqual({ refreshToken: "r-1" });

    await expect(
      revokeRefreshToken("r-1", {
        gatewayUrl,
        fetchFn: vi.fn(async () => {
          throw new TypeError("down");
        }),
      }),
    ).resolves.toBeUndefined();
  });
});
