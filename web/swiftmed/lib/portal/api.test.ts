import { afterEach, describe, expect, it, vi } from "vitest";
import type { PortalSession } from "./api";
import {
  collectHold,
  DashboardApiError,
  getSyncJob,
  getUnmetDemand,
  listPrices,
  setPrice,
  setStock,
  uploadStockCsv,
} from "./api";

const session: PortalSession = { pharmacyId: "ph-1", accessToken: "token-1" };

function mockFetch(status: number, body: unknown) {
  const fetchMock = vi.fn(async () => new Response(body === undefined ? "" : JSON.stringify(body), { status }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function lastCall(fetchMock: ReturnType<typeof mockFetch>): { url: string; init: RequestInit } {
  const [url, init] = fetchMock.mock.calls.at(-1) as unknown as [URL, RequestInit];
  return { url: url.toString(), init };
}

afterEach(() => vi.unstubAllGlobals());

describe("getUnmetDemand", () => {
  it("requests the signed-in pharmacy's report and returns at most ten items", async () => {
    const items = Array.from({ length: 11 }, (_, index) => ({
      productId: `product-${index + 1}`,
      productName: `Medicine ${index + 1}`,
      searchCount: 20 - index,
      unmetCount: 10 - index,
    }));
    const fetchMock = mockFetch(200, { items });

    const result = await getUnmetDemand(session);

    expect(result).toHaveLength(10);
    expect(result[0]?.productName).toBe("Medicine 1");

    const { url, init } = lastCall(fetchMock);
    expect(url).toMatch(/\/api\/v1\/pharmacies\/me\/analytics\/unmet-demand$/);
    expect(init.method).toBe("GET");
    expect(new Headers(init.headers).get("Authorization")).toBe("Bearer token-1");
  });
});

describe("uploadStockCsv", () => {
  it("posts the file as multipart to the pharmacy's Stock Sync jobs", async () => {
    const fetchMock = mockFetch(201, { jobId: "job-1", status: "SYNC_JOB_STATUS_PARTIAL" });
    const file = new File(["product_id,quantity\np-1,4\n"], "stock.csv", { type: "text/csv" });

    const job = await uploadStockCsv(session, file);

    expect(job.jobId).toBe("job-1");
    const { url, init } = lastCall(fetchMock);
    expect(url).toMatch(/\/api\/v1\/pharmacies\/ph-1\/stock-sync\/jobs$/);
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("Authorization")).toBe("Bearer token-1");
    // fetch must set the multipart boundary itself.
    expect(new Headers(init.headers).has("Content-Type")).toBe(false);
    expect(init.body).toBeInstanceOf(FormData);
    expect(((init.body as FormData).get("file") as File).name).toBe("stock.csv");
  });

  it("explains a file that is too large", async () => {
    mockFetch(413, undefined);
    await expect(uploadStockCsv(session, new File(["x"], "big.csv"))).rejects.toMatchObject({
      status: 413,
      message: "The file is larger than 1 MB.",
    });
  });
});

describe("getSyncJob", () => {
  it("reads one job of the signed-in pharmacy", async () => {
    const fetchMock = mockFetch(200, { jobId: "job/1", status: "SYNC_JOB_STATUS_COMPLETED" });
    await getSyncJob(session, "job/1");
    expect(lastCall(fetchMock).url).toMatch(/\/pharmacies\/ph-1\/stock-sync\/jobs\/job%2F1$/);
  });
});

describe("errors", () => {
  it("shows the Gateway's message for client errors", async () => {
    mockFetch(400, { error: "quantity is lower than the reserved quantity" });
    await expect(setStock(session, "p-1", 0)).rejects.toMatchObject({
      status: 400,
      message: "quantity is lower than the reserved quantity",
    });
  });

  it("hides server error details", async () => {
    mockFetch(503, { error: "rpc error: code = Unavailable desc = dial tcp 10.0.0.4" });
    await expect(setStock(session, "p-1", 3)).rejects.toMatchObject({
      status: 503,
      message: "The Gateway could not complete the request.",
    });
  });

  it("reports an unreachable Gateway", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Promise.reject(new TypeError("fetch failed"))));
    await expect(collectHold(session, "h-1", "123456")).rejects.toBeInstanceOf(DashboardApiError);
    await expect(collectHold(session, "h-1", "123456")).rejects.toThrow("Could not reach the API Gateway.");
  });
});

describe("collectHold", () => {
  it("sends the pickup code as JSON", async () => {
    const fetchMock = mockFetch(200, { holdId: "h-1", status: "HOLD_STATUS_COLLECTED" });
    await collectHold(session, "h-1", "482913");
    const { url, init } = lastCall(fetchMock);
    expect(url).toMatch(/\/api\/v1\/holds\/h-1\/collect$/);
    expect(new Headers(init.headers).get("Content-Type")).toBe("application/json");
    expect(JSON.parse(init.body as string)).toEqual({ pickupCode: "482913" });
  });
});

describe("prices (SCRUM-154)", () => {
  it("reads the pharmacy's prices for the listed medicines", async () => {
    const fetchMock = mockFetch(200, { prices: [{ pharmacyId: "ph-1", productId: "p-1", priceChhertum: "4500" }, { productId: "p-9", priceChhertum: "0" }] });
    const prices = await listPrices(session, ["p-1", "p-2", "p-1"]);
    expect([...prices]).toEqual([["p-1", 4500]]);
    const { url, init } = lastCall(fetchMock);
    expect(url).toMatch(/\/api\/v1\/pharmacies\/ph-1\/prices\?productId=p-1&productId=p-2$/);
    expect(init.method).toBe("GET");
  });

  it("asks in batches of 100", async () => {
    const fetchMock = mockFetch(200, { prices: [] });
    await listPrices(session, Array.from({ length: 150 }, (_, i) => `p-${i}`));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("sets one price as a string of chhertum", async () => {
    const fetchMock = mockFetch(200, { prices: [] });
    await setPrice(session, "p-1", 4550);
    const { url, init } = lastCall(fetchMock);
    expect(url).toMatch(/\/api\/v1\/pharmacies\/ph-1\/prices$/);
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ updates: [{ productId: "p-1", priceChhertum: "4550" }] });
  });

  it("passes the gateway's message on a refusal", async () => {
    mockFetch(400, { error: "priceChhertum must be a whole number of chhertum from 1 to 100000000" });
    await expect(setPrice(session, "p-1", 0)).rejects.toBeInstanceOf(DashboardApiError);
  });
});
