import { describe, expect, it, vi } from "vitest";

import { DemoHolds } from "./demo-holds";
import { HoldError, errorForStatus, gatewayBackend, holdsBackend } from "./holds";
import { errorMessage } from "./hold-format";

const session = { userId: "u-1", email: "tashi@example.bt", accessToken: "tok-1" };
const hold = { holdId: "h-1", status: "HOLD_STATUS_PENDING", quantity: 2 };

function json(status: number, body?: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function backendReturning(...responses: (Response | Error)[]) {
  const fetchFn = vi.fn<typeof fetch>();
  for (const r of responses) {
    if (r instanceof Error) fetchFn.mockRejectedValueOnce(r);
    else fetchFn.mockResolvedValueOnce(r);
  }
  return { backend: gatewayBackend({ gatewayUrl: "http://gw:8080", fetchFn }), fetchFn };
}

const requestOf = (fetchFn: ReturnType<typeof vi.fn<typeof fetch>>, call = 0) => {
  const [url, init] = fetchFn.mock.calls[call];
  const headers = (init?.headers ?? {}) as Record<string, string>;
  return { url: String(url), method: init?.method, headers, body: init?.body ? JSON.parse(String(init.body)) : undefined };
};

async function kind(promise: Promise<unknown>) {
  try {
    await promise;
    return "no error";
  } catch (err) {
    return err instanceof HoldError ? err.kind : String(err);
  }
}

describe("holdsBackend", () => {
  it("uses the demo without a gateway and the gateway with one", () => {
    expect(holdsBackend("")).toBeInstanceOf(DemoHolds);
    expect(holdsBackend("http://gw:8080").source).toBe("gateway");
  });
});

describe("gateway backend", () => {
  it("shows a useful message when sign-in is rate limited", async () => {
    const { backend } = backendReturning(json(429, { error: "too many authentication attempts; try again later" }));
    try {
      await backend.login("tashi@example.bt", "wrong-password");
      throw new Error("expected sign-in to be rate limited");
    } catch (err) {
      expect(err).toBeInstanceOf(HoldError);
      expect(errorMessage((err as HoldError).kind)).toMatch(/too many sign-in attempts/i);
    }
  });

  it("registers a patient", async () => {
    const { backend, fetchFn } = backendReturning(json(201, { userId: "u-1" }));
    await backend.register("tashi@example.bt", "correct-horse");
    const req = requestOf(fetchFn);
    expect(req.url).toBe("http://gw:8080/api/v1/auth/register");
    expect(req.method).toBe("POST");
    expect(req.body).toEqual({ email: "tashi@example.bt", password: "correct-horse", role: "ROLE_PATIENT" });
  });

  it("signs in and keeps the access token", async () => {
    const { backend } = backendReturning(
      json(200, { userId: "u-1", role: "ROLE_PATIENT", tokens: { accessToken: "a", refreshToken: "r", accessTokenExpiresAt: "2026-11-05T09:15:00Z" } }),
    );
    expect(await backend.login("tashi@example.bt", "pw-123456")).toEqual({
      userId: "u-1",
      email: "tashi@example.bt",
      accessToken: "a",
      expiresAt: "2026-11-05T09:15:00Z",
      refreshToken: "r",
      role: "ROLE_PATIENT",
    });
  });

  it("keeps a staff member's role and pharmacy from the token (SCRUM-139)", async () => {
    // A JWT whose payload carries pharmacy_id; the signature is not checked here.
    const payload = Buffer.from(JSON.stringify({ sub: "s-1", pharmacy_id: "ph-1" })).toString("base64url");
    const { backend } = backendReturning(json(200, { userId: "s-1", role: "ROLE_PHARMACY_STAFF", tokens: { accessToken: `h.${payload}.s` } }));
    expect(await backend.login("demo-tim-001@demo.swiftmeds.test", "pw-123456")).toMatchObject({ role: "ROLE_PHARMACY_STAFF", pharmacyId: "ph-1" });
  });

  it("assumes the default lifetime when the login answer has no expiry", async () => {
    const { backend } = backendReturning(json(200, { userId: "u-1", tokens: { accessToken: "a" } }));
    const before = Date.now();
    const session = await backend.login("tashi@example.bt", "pw-123456");
    expect(session.refreshToken).toBeUndefined();
    const ttl = Date.parse(session.expiresAt ?? "") - before;
    expect(ttl).toBeGreaterThanOrEqual(15 * 60 * 1000 - 1000);
    expect(ttl).toBeLessThanOrEqual(15 * 60 * 1000 + 1000);
  });

  it("rejects a login answer without a token", async () => {
    const { backend } = backendReturning(json(200, { userId: "u-1" }));
    expect(await kind(backend.login("a@b.bt", "pw-123456"))).toBe("server");
  });

  it("creates, reads, lists and cancels holds with the bearer token", async () => {
    const { backend, fetchFn } = backendReturning(
      json(202, hold),
      json(200, { ...hold, status: "HOLD_STATUS_ACTIVE", pickupCode: "123456" }),
      json(200, { holds: [hold], page: {} }),
      json(200, { ...hold, status: "HOLD_STATUS_CANCELLED" }),
    );

    expect((await backend.createHold(session, { pharmacyId: "ph", productId: "pr", quantity: 2 })).holdId).toBe("h-1");
    expect((await backend.getHold(session, "h-1"))?.pickupCode).toBe("123456");
    expect(await backend.listHolds(session)).toHaveLength(1);
    expect((await backend.cancelHold(session, "h-1")).status).toBe("HOLD_STATUS_CANCELLED");

    const [create, get, list, cancel] = [0, 1, 2, 3].map((i) => requestOf(fetchFn, i));
    expect(create).toMatchObject({ url: "http://gw:8080/api/v1/holds", method: "POST", body: { pharmacyId: "ph", productId: "pr", quantity: 2 } });
    expect(get).toMatchObject({ url: "http://gw:8080/api/v1/holds/h-1", method: "GET" });
    expect(list.url).toBe("http://gw:8080/api/v1/holds?pageSize=50");
    expect(cancel).toMatchObject({ url: "http://gw:8080/api/v1/holds/h-1/cancel", method: "POST" });
    for (const req of [create, get, list, cancel]) expect(req.headers.Authorization).toBe("Bearer tok-1");
  });

  it("treats a missing hold as not found", async () => {
    const { backend } = backendReturning(json(404, { error: "not found" }));
    expect(await backend.getHold(session, "nope")).toBeUndefined();
  });

  it("escapes the hold ID in the path", async () => {
    const { backend, fetchFn } = backendReturning(json(404, { error: "not found" }));
    await backend.getHold(session, "a/../b");
    expect(requestOf(fetchFn).url).toBe("http://gw:8080/api/v1/holds/a%2F..%2Fb");
  });

  it("passes other errors on", async () => {
    const { backend } = backendReturning(json(503, { error: "down" }));
    expect(await kind(backend.getHold(session, "h-1"))).toBe("server");
  });

  it("rejects a hold without an ID or status", async () => {
    const { backend } = backendReturning(json(202, { status: "HOLD_STATUS_PENDING" }));
    expect(await kind(backend.createHold(session, { pharmacyId: "ph", productId: "pr", quantity: 1 }))).toBe("server");
  });

  it("explains network failures and timeouts", async () => {
    const timeout = Object.assign(new Error("timed out"), { name: "TimeoutError" });
    const { backend } = backendReturning(new TypeError("fetch failed"), timeout);
    expect(await kind(backend.listHolds(session))).toBe("unreachable");
    expect(await kind(backend.listHolds(session))).toBe("timeout");
  });

  it("explains an answer that is not JSON", async () => {
    const { backend } = backendReturning(new Response("<html>", { status: 200 }));
    expect(await kind(backend.listHolds(session))).toBe("server");
  });
});

describe("errorForStatus", () => {
  it.each([
    [400, "hold", "bad_request"],
    [401, "login", "invalid_credentials"],
    [401, "hold", "unauthorized"],
    [403, "hold", "unauthorized"],
    [404, "hold", "not_found"],
    [404, "login", "not_available"],
    [409, "register", "email_taken"],
    [409, "hold", "not_active"],
    [412, "hold", "not_active"],
    [501, "login", "not_available"],
    [504, "hold", "timeout"],
    [500, "hold", "server"],
  ] as const)("HTTP %i on %s is %s", (status, action, want) => {
    expect(errorForStatus(status, action).kind).toBe(want);
  });
});
