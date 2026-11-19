import { describe, expect, it } from "vitest";

import {
  DEFAULT_SESSION_SECONDS,
  REFRESH_SESSION_SECONDS,
  decodeSession,
  encodeSession,
  needsRefresh,
  parseSession,
  safeNext,
  sessionMaxAge,
} from "./session";

const now = Date.parse("2026-11-05T09:00:00Z");
const session = { userId: "u-1", email: "tashi@example.bt", accessToken: "token-1" };

describe("session cookie", () => {
  it("round-trips a session", () => {
    expect(decodeSession(encodeSession(session), now)).toEqual(session);
    const withExpiry = { ...session, expiresAt: "2026-11-05T09:15:00Z" };
    expect(decodeSession(encodeSession(withExpiry), now)).toEqual(withExpiry);
  });

  it("rejects missing, malformed, incomplete and expired values", () => {
    expect(decodeSession(undefined, now)).toBeUndefined();
    expect(decodeSession("", now)).toBeUndefined();
    expect(decodeSession("not base64 json", now)).toBeUndefined();
    expect(decodeSession(Buffer.from("[1,2]").toString("base64url"), now)).toBeUndefined();
    expect(decodeSession(encodeSession({ ...session, accessToken: "" }), now)).toBeUndefined();
    expect(decodeSession(encodeSession({ ...session, expiresAt: "2026-11-05T08:59:59Z" }), now)).toBeUndefined();
    expect(decodeSession(encodeSession({ ...session, expiresAt: "not a date" }), now)).toBeUndefined();
  });

  it("lives as long as the access token, or the default", () => {
    expect(sessionMaxAge(session, now)).toBe(DEFAULT_SESSION_SECONDS);
    expect(sessionMaxAge({ ...session, expiresAt: "2026-11-05T09:10:00Z" }, now)).toBe(600);
    expect(sessionMaxAge({ ...session, expiresAt: "2026-11-05T08:00:00Z" }, now)).toBe(0);
  });

  it("keeps the refresh token, and outlives the access token when there is one", () => {
    const withRefresh = { ...session, expiresAt: "2026-11-05T09:15:00Z", refreshToken: "r-1" };
    expect(decodeSession(encodeSession(withRefresh), now)).toEqual(withRefresh);
    expect(sessionMaxAge(withRefresh, now)).toBe(REFRESH_SESSION_SECONDS);
    expect(decodeSession(encodeSession({ ...withRefresh, refreshToken: "" }), now)).toBeUndefined();
  });

  it("parses an expired session for refreshing, but does not treat it as signed in", () => {
    const expired = { ...session, expiresAt: "2026-11-05T08:59:00Z", refreshToken: "r-1" };
    expect(parseSession(encodeSession(expired))).toEqual(expired);
    expect(decodeSession(encodeSession(expired), now)).toBeUndefined();
  });
});

describe("needsRefresh", () => {
  const s = { ...session, expiresAt: "2026-11-05T09:15:00Z", refreshToken: "r-1" };
  it("is true within the margin before expiry and after it", () => {
    expect(needsRefresh(s, Date.parse("2026-11-05T09:13:59Z"))).toBe(false);
    expect(needsRefresh(s, Date.parse("2026-11-05T09:14:00Z"))).toBe(true);
    expect(needsRefresh(s, Date.parse("2026-11-05T10:00:00Z"))).toBe(true);
  });
  it("is false without a refresh token or an expiry", () => {
    expect(needsRefresh({ ...s, refreshToken: undefined }, Date.parse("2026-11-05T10:00:00Z"))).toBe(false);
    expect(needsRefresh({ ...s, expiresAt: undefined }, Date.parse("2026-11-05T10:00:00Z"))).toBe(false);
  });
});

describe("safeNext", () => {
  it("keeps same-site paths", () => {
    expect(safeNext("/reserve?product=p&pharmacy=x")).toBe("/reserve?product=p&pharmacy=x");
  });

  it("falls back for anything that could leave the site", () => {
    for (const bad of [undefined, null, "", "https://evil.example", "//evil.example", "/\\evil.example", "holds"]) {
      expect(safeNext(bad)).toBe("/holds");
    }
  });
});
