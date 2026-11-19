import { describe, expect, it, vi } from "vitest";

import { checkProfile, demoProfile, gatewayProfile, initialOf } from "./profile";
import type { Session } from "./types";

const session: Session = { userId: "u1", email: "patient@demo.swiftmeds.test", accessToken: "token-1" };

describe("checkProfile", () => {
  it("trims the name and removes the separators people type in phone numbers", () => {
    expect(checkProfile({ displayName: "  Karma  ", phone: "+975 17-12 34 56" })).toEqual({ profile: { displayName: "Karma", phone: "+97517123456" } });
    expect(checkProfile({ displayName: "Karma", phone: "(17) 123.456" })).toEqual({ profile: { displayName: "Karma", phone: "17123456" } });
  });

  it.each([
    [{ displayName: " ", phone: "17123456" }, "Enter your name."],
    [{ displayName: "a".repeat(81), phone: "17123456" }, "at most 80"],
    [{ displayName: "Karma", phone: "1712" }, "8 to 15 digits"],
    [{ displayName: "Karma", phone: "" }, "8 to 15 digits"],
    [{ displayName: "Karma", phone: "01712345" }, "8 to 15 digits"],
  ])("says what to fix for %o", (input, message) => {
    const result = checkProfile(input);
    expect("error" in result && result.error).toContain(message);
  });
});

describe("initialOf", () => {
  it("uses the name, then the email", () => {
    expect(initialOf("karma", "x@y.z")).toBe("K");
    expect(initialOf(undefined, "patient@demo.swiftmeds.test")).toBe("P");
    expect(initialOf("  ", "ünal@demo.test")).toBe("Ü");
  });
});

describe("gatewayProfile", () => {
  const backend = (fetchFn: typeof fetch) => gatewayProfile({ gatewayUrl: "http://gw.test", fetchFn });

  it("reads the profile with the patient's token", async () => {
    const fetchFn = vi.fn(async () => Response.json({ userId: "u1", displayName: "Karma", phone: "17123456" }));
    expect(await backend(fetchFn).get(session)).toEqual({ displayName: "Karma", phone: "17123456" });
    const [url, init] = fetchFn.mock.calls[0] as unknown as [URL, RequestInit];
    expect(String(url)).toBe("http://gw.test/api/v1/me/profile");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer token-1");
  });

  it("treats 404 as no profile yet", async () => {
    expect(await backend(vi.fn(async () => new Response(null, { status: 404 }))).get(session)).toBeUndefined();
  });

  it("saves with PUT and a JSON body", async () => {
    const fetchFn = vi.fn(async () => Response.json({ displayName: "Karma", phone: "17123456" }));
    await backend(fetchFn).save(session, { displayName: "Karma", phone: "17123456" });
    const [, init] = fetchFn.mock.calls[0] as unknown as [URL, RequestInit];
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ displayName: "Karma", phone: "17123456" });
  });

  it("shows the gateway's own sentence when it rejects the input", async () => {
    const fetchFn = vi.fn(async () => Response.json({ error: "enter your name" }, { status: 400 }));
    await expect(backend(fetchFn).save(session, { displayName: "", phone: "1" })).rejects.toMatchObject({ kind: "bad_request", message: "Enter your name." });
  });

  it("reports an ended session as unauthorized", async () => {
    await expect(backend(vi.fn(async () => new Response(null, { status: 401 }))).get(session)).rejects.toMatchObject({ kind: "unauthorized" });
  });
});

describe("demoProfile", () => {
  it("keeps what was saved for that user only", async () => {
    const demo = demoProfile();
    await demo.save(session, { displayName: " Karma ", phone: "17123456" });
    expect(await demo.get(session)).toEqual({ displayName: "Karma", phone: "17123456" });
    expect(await demo.get({ ...session, userId: "someone-else" })).toBeUndefined();
    await expect(demo.save(session, { displayName: "", phone: "17123456" })).rejects.toMatchObject({ kind: "bad_request" });
  });
});
