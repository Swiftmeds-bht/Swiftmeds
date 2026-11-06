import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { callSweep, SECRET_HEADER, SWEEP_PATH, type Env } from "../src/sweep";
import worker from "../src/index";

const env: Env = {
  GATEWAY_URL: "https://gateway.example.test",
  WORKER_SHARED_SECRET: "test-worker-secret-0123456789",
};

const fixedNow = () => new Date(Date.UTC(2026, 10, 5, 7, 0, 0, 123));

const HOLD_A = "11111111-1111-4111-8111-111111111111";
const HOLD_B = "22222222-2222-4222-8222-222222222222";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function fakeFetch(response: Response | Error) {
  return vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
    if (response instanceof Error) {
      throw response;
    }
    return response;
  });
}

type FakeFetch = ReturnType<typeof fakeFetch>;

function deps(fetchMock: FakeFetch) {
  return { fetchFn: fetchMock as unknown as typeof fetch, now: fixedNow };
}

describe("callSweep", () => {
  it("POSTs asOf with the secret header to the sweep endpoint and returns the expired hold IDs", async () => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [HOLD_A, HOLD_B] }));

    const ids = await callSweep(env, deps(fetchMock));

    expect(ids).toEqual([HOLD_A, HOLD_B]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`https://gateway.example.test${SWEEP_PATH}`);
    expect(init?.method).toBe("POST");
    const headers = init?.headers as Record<string, string>;
    expect(headers[SECRET_HEADER]).toBe(env.WORKER_SHARED_SECRET);
    expect(headers["Content-Type"]).toBe("application/json");
    expect(init?.body).toBe('{"asOf":"2026-11-05T07:00:00.123Z"}');
    expect(init?.signal).toBeDefined();
  });

  it("does not use the Authorization header, which is for user tokens", async () => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [] }));

    await callSweep(env, deps(fetchMock));

    const headers = fetchMock.mock.calls[0]![1]?.headers as Record<string, string>;
    expect(headers.Authorization).toBe(undefined);
  });

  it("treats a missing expiredHoldIds as none expired, because protobuf JSON leaves out an empty list", async () => {
    const fetchMock = fakeFetch(jsonResponse({}));
    await expect(callSweep(env, deps(fetchMock))).resolves.toEqual([]);
  });

  it("accepts an explicit empty list", async () => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [] }));
    await expect(callSweep(env, deps(fetchMock))).resolves.toEqual([]);
  });

  it("ignores trailing slashes and spaces in GATEWAY_URL", async () => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [] }));

    await callSweep({ ...env, GATEWAY_URL: " https://gateway.example.test/// " }, deps(fetchMock));

    expect(fetchMock.mock.calls[0]![0]).toBe(`https://gateway.example.test${SWEEP_PATH}`);
  });

  it.each([
    ["GATEWAY_URL", { ...env, GATEWAY_URL: "" }],
    ["GATEWAY_URL", { ...env, GATEWAY_URL: "   " }],
    ["WORKER_SHARED_SECRET", { ...env, WORKER_SHARED_SECRET: "" }],
    ["WORKER_SHARED_SECRET", { ...env, WORKER_SHARED_SECRET: "  " }],
  ])("fails without calling the gateway when %s is missing", async (name, badEnv) => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [] }));

    await expect(callSweep(badEnv, deps(fetchMock))).rejects.toThrow(name);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([400, 401, 403, 404, 500, 503, 504])("throws when the gateway answers HTTP %i", async (status) => {
    const fetchMock = fakeFetch(jsonResponse({ error: "nope" }, status));

    await expect(callSweep(env, deps(fetchMock))).rejects.toThrow(`HTTP ${status}`);
  });

  it("never puts the secret in an error message", async () => {
    const fetchMock = fakeFetch(jsonResponse({ error: "unauthorized" }, 401));

    const error = await callSweep(env, deps(fetchMock)).catch((e: Error) => e);

    expect((error as Error).message).not.toContain(env.WORKER_SHARED_SECRET);
  });

  it("throws when the gateway cannot be reached", async () => {
    const fetchMock = fakeFetch(new TypeError("fetch failed"));

    await expect(callSweep(env, deps(fetchMock))).rejects.toThrow("fetch failed");
  });

  it.each([
    ["a body that is not JSON", () => new Response("<html>oops</html>", { status: 200 }), "not a JSON object"],
    ["a JSON array", () => jsonResponse([1, 2]), "not a JSON object"],
    ["JSON null", () => jsonResponse(null), "not a JSON object"],
    ["an expiredHoldIds that is not a list", () => jsonResponse({ expiredHoldIds: HOLD_A }), "invalid expiredHoldIds"],
    ["an expiredHoldIds with a non-string", () => jsonResponse({ expiredHoldIds: [HOLD_A, 7] }), "invalid expiredHoldIds"],
  ])("throws on %s", async (_label, makeResponse, message) => {
    const fetchMock = fakeFetch(makeResponse());

    await expect(callSweep(env, deps(fetchMock))).rejects.toThrow(message);
  });
});

describe("scheduled handler", () => {
  const controller = {
    cron: "* * * * *",
    scheduledTime: Date.UTC(2026, 10, 5, 7, 0, 0),
    noRetry() {},
  } as unknown as ScheduledController;

  beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("calls the gateway and logs how many holds were expired", async () => {
    const fetchMock = fakeFetch(jsonResponse({ expiredHoldIds: [HOLD_A, HOLD_B] }));
    vi.stubGlobal("fetch", fetchMock);

    await worker.scheduled(controller, env);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const logged = JSON.parse(vi.mocked(console.log).mock.calls[0]![0] as string);
    expect(logged).toMatchObject({ level: "info", expiredCount: 2, cron: "* * * * *" });
    expect(logged.expiredHoldIds).toEqual([HOLD_A, HOLD_B]);
  });

  it("lists at most 20 hold IDs in the log but counts them all", async () => {
    const many = Array.from({ length: 25 }, (_, i) => `hold-${i}`);
    vi.stubGlobal("fetch", fakeFetch(jsonResponse({ expiredHoldIds: many })));

    await worker.scheduled(controller, env);

    const logged = JSON.parse(vi.mocked(console.log).mock.calls[0]![0] as string);
    expect(logged.expiredCount).toBe(25);
    expect(logged.expiredHoldIds.length).toBe(20);
  });

  it("logs and rethrows when the gateway rejects the secret", async () => {
    vi.stubGlobal("fetch", fakeFetch(jsonResponse({ error: "unauthorized" }, 401)));

    await expect(worker.scheduled(controller, env)).rejects.toThrow("HTTP 401");

    const logged = JSON.parse(vi.mocked(console.error).mock.calls[0]![0] as string);
    expect(logged).toMatchObject({ level: "error", msg: "hold expiry sweep failed" });
    expect(JSON.stringify(logged)).not.toContain(env.WORKER_SHARED_SECRET);
  });
});
