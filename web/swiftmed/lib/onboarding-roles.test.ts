// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

import { checkPharmacyForm, DZONGKHAGS, EMPTY_PHARMACY_FORM, type PharmacyFormValues } from "./pharmacy-form";
import { forgetRefreshes, refreshSession } from "./refresh";
import { ADMIN_PATH, claimsFromToken, homeAfterSignIn, isAdmin, isStaffAccount, isAdminPath } from "./roles";
import type { Session } from "./types";

/** An unsigned JWT with these claims: the app only reads them; the gateway verifies. */
const jwt = (claims: Record<string, unknown>) =>
  `x.${Buffer.from(JSON.stringify(claims), "utf8").toString("base64url")}.y`;

describe("roles for pharmacy sign-up and admins (SCRUM-142)", () => {
  it("reads Auth's role names from the token as the API names them", () => {
    expect(claimsFromToken(jwt({ role: "PHARMACIST", pharmacy_id: "ph-9" }))).toEqual({ role: "ROLE_PHARMACY_STAFF", pharmacyId: "ph-9" });
    expect(claimsFromToken(jwt({ role: "PATIENT", pharmacy_id: "" }))).toEqual({ role: "ROLE_PATIENT" });
    expect(claimsFromToken(jwt({ role: "ADMIN" }))).toEqual({ role: "ROLE_ADMIN" });
    expect(claimsFromToken(jwt({ role: "SOMETHING" }))).toEqual({});
    expect(claimsFromToken("not-a-jwt")).toEqual({});
    expect(claimsFromToken("x.%%%.y")).toEqual({});
  });

  it("tells admins, staff accounts and patients apart", () => {
    expect(isAdmin({ role: "ROLE_ADMIN" })).toBe(true);
    expect(isAdmin({ role: "ROLE_ADMIN", pharmacyId: "ph-1" })).toBe(false); // an admin linked to a pharmacy uses the portal
    expect(isStaffAccount({ role: "ROLE_PHARMACY_STAFF" })).toBe(true); // before its pharmacy is linked
    expect(isStaffAccount({ role: "ROLE_PATIENT" })).toBe(false);
    expect(isStaffAccount(undefined)).toBe(false);
  });

  it("sends admins to /admin, staff to the portal and keeps patients out of both", () => {
    const admin = { role: "ROLE_ADMIN" };
    expect(homeAfterSignIn(admin, "/holds")).toBe(ADMIN_PATH);
    expect(homeAfterSignIn(admin, "/admin?status=verified")).toBe("/admin?status=verified");
    expect(homeAfterSignIn({ role: "ROLE_PHARMACY_STAFF" }, "/holds")).toBe("/pharmacy-portal");
    expect(homeAfterSignIn({ role: "ROLE_PATIENT" }, "/admin")).toBe("/");
    expect(isAdminPath("/administrator")).toBe(false);
  });
});

describe("refresh after registering a pharmacy (SCRUM-141, SCRUM-142)", () => {
  afterEach(() => forgetRefreshes());

  it("takes the new role and pharmacy from the refreshed token", async () => {
    const session: Session = { userId: "u-1", email: "owner@example.bt", accessToken: "old", refreshToken: "r-1", role: "ROLE_PATIENT" };
    const accessToken = jwt({ role: "PHARMACIST", pharmacy_id: "ph-new" });
    const fetchFn = vi.fn(async () =>
      new Response(JSON.stringify({ tokens: { accessToken, refreshToken: "r-2", accessTokenExpiresAt: "2026-11-20T10:00:00Z" } }), { status: 200 }),
    );
    const outcome = await refreshSession(session, { gatewayUrl: "http://gateway:8080", fetchFn });
    expect(outcome.kind).toBe("refreshed");
    if (outcome.kind !== "refreshed") return;
    expect(outcome.session.role).toBe("ROLE_PHARMACY_STAFF");
    expect(outcome.session.pharmacyId).toBe("ph-new");
  });
});

const filled = (over: Partial<PharmacyFormValues> = {}): PharmacyFormValues => ({
  ...EMPTY_PHARMACY_FORM,
  name: "Thimphu Care",
  licenseNumber: "BMHC-114",
  phone: "02 334 455",
  address: "Norzin Lam",
  dzongkhag: "Thimphu",
  ...over,
});

describe("checkPharmacyForm", () => {
  it("places the pharmacy at its dzongkhag's centre unless the device gave a location", () => {
    const thimphu = DZONGKHAGS.find((d) => d.name === "Thimphu")!.centre;
    expect(checkPharmacyForm(filled())).toEqual({
      details: { name: "Thimphu Care", licenseNumber: "BMHC-114", phone: "02334455", address: "Norzin Lam", dzongkhag: "Thimphu", location: thimphu },
    });
    const own = checkPharmacyForm(filled({ latitude: "27.4801", longitude: "89.6412" }));
    expect("details" in own && own.details.location).toEqual({ latitude: 27.4801, longitude: 89.6412 });
  });

  it.each([
    [{ name: "" }, "name"],
    [{ licenseNumber: "" }, "licence number"],
    [{ phone: "12" }, "phone number"],
    [{ address: "" }, "street address"],
    [{ dzongkhag: "Atlantis" }, "dzongkhag"],
    [{ latitude: "95", longitude: "89" }, "location"],
    [{ name: "x".repeat(201) }, "too long"],
  ])("says what to fix: %o", (over, words) => {
    const result = checkPharmacyForm(filled(over));
    expect("error" in result && result.error.toLowerCase()).toContain(words);
  });

  it("knows all 20 dzongkhags, spelled as in the seed data", () => {
    expect(DZONGKHAGS).toHaveLength(20);
    expect(DZONGKHAGS.map((d) => d.name)).toContain("Chukha");
  });
});
