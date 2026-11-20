import { describe, expect, it } from "vitest";

import { homeAfterSignIn, isPharmacyStaff, pharmacyIdFromToken } from "./roles";
import { decodeSession, encodeSession } from "./session";

const staff = { role: "ROLE_PHARMACY_STAFF", pharmacyId: "ph-1" };
const patient = { role: "ROLE_PATIENT" };

describe("isPharmacyStaff", () => {
  it("is staff, or an admin, linked to a pharmacy", () => {
    expect(isPharmacyStaff(staff)).toBe(true);
    expect(isPharmacyStaff({ role: "ROLE_ADMIN", pharmacyId: "ph-1" })).toBe(true);
    expect(isPharmacyStaff({ role: "ROLE_ADMIN" })).toBe(false); // no pharmacy to manage
    expect(isPharmacyStaff({ role: "ROLE_PHARMACY_STAFF" })).toBe(false);
    expect(isPharmacyStaff(patient)).toBe(false);
    expect(isPharmacyStaff(undefined)).toBe(false);
  });
});

describe("homeAfterSignIn", () => {
  it("sends staff to the portal, keeping a portal page they asked for", () => {
    expect(homeAfterSignIn(staff, "/holds")).toBe("/pharmacy-portal");
    expect(homeAfterSignIn(staff, "/pharmacy-portal/stock")).toBe("/pharmacy-portal/stock");
  });

  it("sends patients where they were going, but never into the portal", () => {
    expect(homeAfterSignIn(patient, "/holds")).toBe("/holds");
    expect(homeAfterSignIn(patient, "/pharmacy-portal")).toBe("/");
    expect(homeAfterSignIn(patient, "/pharmacy/p1")).toBe("/pharmacy/p1"); // a patient page, not the portal
  });
});

describe("pharmacyIdFromToken", () => {
  const token = (claims: object) => `h.${Buffer.from(JSON.stringify(claims)).toString("base64url")}.s`;
  it("reads the pharmacy_id claim, and nothing from a malformed token", () => {
    expect(pharmacyIdFromToken(token({ pharmacy_id: "ph-1" }))).toBe("ph-1");
    expect(pharmacyIdFromToken(token({ sub: "u" }))).toBeUndefined();
    expect(pharmacyIdFromToken("not-a-jwt")).toBeUndefined();
    expect(pharmacyIdFromToken("h.%%%.s")).toBeUndefined();
  });
});

describe("session cookie", () => {
  it("keeps the role and pharmacy across encode and decode", () => {
    const value = encodeSession({ userId: "s-1", email: "s@x.test", accessToken: "t", ...staff });
    expect(decodeSession(value)).toMatchObject(staff);
  });
});
