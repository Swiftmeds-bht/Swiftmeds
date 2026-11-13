import { describe, expect, it } from "vitest";

import { PHARMACY_STAFF_ROLE, sessionFromLogin } from "./session";

function jwt(payload: object): string {
  const part = (value: object) => Buffer.from(JSON.stringify(value)).toString("base64url");
  return `${part({ alg: "none" })}.${part(payload)}.sig`;
}

const future = new Date(Date.now() + 15 * 60_000).toISOString();
const login = (overrides: object = {}) => ({
  userId: "u-1",
  role: PHARMACY_STAFF_ROLE,
  tokens: { accessToken: jwt({ pharmacy_id: "ph-1" }), refreshToken: "r-1", accessTokenExpiresAt: future },
  ...overrides,
});

describe("sessionFromLogin", () => {
  it("builds a session for pharmacy staff, with the pharmacy from the token", () => {
    expect(sessionFromLogin(login())).toMatchObject({
      userId: "u-1",
      role: PHARMACY_STAFF_ROLE,
      pharmacyId: "ph-1",
      refreshToken: "r-1",
      expiresAt: future,
    });
  });

  it("refuses patients and admins", () => {
    expect(() => sessionFromLogin(login({ role: "ROLE_PATIENT" }))).toThrow(/pharmacy staff access/);
    expect(() => sessionFromLogin(login({ role: "ROLE_ADMIN" }))).toThrow(/pharmacy staff access/);
  });

  it("refuses staff accounts not linked to a pharmacy", () => {
    const tokens = { accessToken: jwt({ sub: "u-1" }), refreshToken: "r-1" };
    expect(() => sessionFromLogin(login({ tokens }))).toThrow(/not linked to a pharmacy/);
  });

  it("refuses broken or expired tokens", () => {
    expect(() => sessionFromLogin(login({ tokens: { accessToken: "nope", refreshToken: "r" } }))).toThrow(/invalid access token/);
    const expired = { accessToken: jwt({ pharmacy_id: "ph-1" }), refreshToken: "r", accessTokenExpiresAt: "2020-01-01T00:00:00Z" };
    expect(() => sessionFromLogin(login({ tokens: expired }))).toThrow(/expired/);
  });
});
