import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { callDecay, DECAY_PATH, SECRET_HEADER, type Env } from "../src/decay";
import worker from "../src/index";

const env: Env = {
  GATEWAY_URL: "https://gateway.example.test",
  WORKER_SHARED_SECRET: "test-worker-secret-0123456789",
};

const fixedNow = () => new Date(Date.UTC(2026, 10, 5, 7, 0, 0, 123));

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

describe("callDecay", () => {
  it("POSTs asOf with the secret header to the decay endpoint and returns updatedCount", async () => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 3 }));

    const updated = await callDecay(env, deps(fetchMock));

    expect(updated).toBe(3);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`https://gateway.example.test${DECAY_PATH}`);
    expect(init?.method).toBe("POST");
    const headers = init?.headers as Record<string, string>;
    expect(headers[SECRET_HEADER]).toBe(env.WORKER_SHARED_SECRET);
    expect(headers["Content-Type"]).toBe("application/json");
    expect(init?.body).toBe('{"asOf":"2026-11-05T07:00:00.123Z"}');
    expect(init?.signal).toBeDefined();
  });

  it("does not use the Authorization header, which is for user tokens", async () => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 1 }));

    await callDecay(env, deps(fetchMock));

    const headers = fetchMock.mock.calls[0]![1]?.headers as Record<string, string>;
    expect(headers.Authorization).toBe(undefined);
  });

  it("treats a missing updatedCount as zero, because protobuf JSON leaves out a zero", async () => {
    const fetchMock = fakeFetch(jsonResponse({}));
    await expect(callDecay(env, deps(fetchMock))).resolves.toBe(0);
  });

  it("accepts an explicit zero", async () => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 0 }));
    await expect(callDecay(env, deps(fetchMock))).resolves.toBe(0);
  });

  it("ignores trailing slashes and spaces in GATEWAY_URL", async () => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 1 }));

    await callDecay({ ...env, GATEWAY_URL: " https://gateway.example.test/// " }, deps(fetchMock));

    expect(fetchMock.mock.calls[0]![0]).toBe(`https://gateway.example.test${DECAY_PATH}`);
  });

  it.each([
    ["GATEWAY_URL", { ...env, GATEWAY_URL: "" }],
    ["GATEWAY_URL", { ...env, GATEWAY_URL: "   " }],
    ["WORKER_SHARED_SECRET", { ...env, WORKER_SHARED_SECRET: "" }],
    ["WORKER_SHARED_SECRET", { ...env, WORKER_SHARED_SECRET: "  " }],
  ])("fails without calling the gateway when %s is missing", async (name, badEnv) => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 1 }));

    await expect(callDecay(badEnv, deps(fetchMock))).rejects.toThrow(name);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([400, 401, 403, 404, 500, 503, 504])("throws when the gateway answers HTTP %i", async (status) => {
    const fetchMock = fakeFetch(jsonResponse({ error: "nope" }, status));

    await expect(callDecay(env, deps(fetchMock))).rejects.toThrow(`HTTP ${status}`);
  });

  it("never puts the secret in an error message", async () => {
    const fetchMock = fakeFetch(jsonResponse({ error: "unauthorized" }, 401));

    const error = await callDecay(env, deps(fetchMock)).catch((e: Error) => e);

    expect((error as Error).message).not.toContain(env.WORKER_SHARED_SECRET);
  });

  it("throws when the gateway cannot be reached", async () => {
    const fetchMock = fakeFetch(new TypeError("fetch failed"));

    await expect(callDecay(env, deps(fetchMock))).rejects.toThrow("fetch failed");
  });

  it.each([
    ["a body that is not JSON", () => new Response("<html>oops</html>", { status: 200 }), "not a JSON object"],
    ["a JSON array", () => jsonResponse([1, 2]), "not a JSON object"],
    ["JSON null", () => jsonResponse(null), "not a JSON object"],
    ["an updatedCount that is not a number", () => jsonResponse({ updatedCount: "3" }), "invalid updatedCount"],
  ])("throws on %s", async (_label, makeResponse, message) => {
    const fetchMock = fakeFetch(makeResponse());

    await expect(callDecay(env, deps(fetchMock))).rejects.toThrow(message);
  });
});

describe("scheduled handler", () => {
  const controller = {
    cron: "0 * * * *",
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

  it("calls the gateway and logs the number of rows changed", async () => {
    const fetchMock = fakeFetch(jsonResponse({ updatedCount: 5 }));
    vi.stubGlobal("fetch", fetchMock);

    await worker.scheduled(controller, env);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const logged = JSON.parse(vi.mocked(console.log).mock.calls[0]![0] as string);
    expect(logged).toMatchObject({ level: "info", updatedCount: 5, cron: "0 * * * *" });
  });

  it("logs and rethrows when the gateway rejects the secret", async () => {
    vi.stubGlobal("fetch", fakeFetch(jsonResponse({ error: "unauthorized" }, 401)));

    await expect(worker.scheduled(controller, env)).rejects.toThrow("HTTP 401");

    const logged = JSON.parse(vi.mocked(console.error).mock.calls[0]![0] as string);
    expect(logged).toMatchObject({ level: "error", msg: "freshness decay failed" });
    expect(JSON.stringify(logged)).not.toContain(env.WORKER_SHARED_SECRET);
  });
});
