import { describe, expect, it, vi } from "vitest";

import { HoldError } from "./holds-core";
import { MOCK_PHARMACIES, MOCK_PRODUCTS } from "./mock-data";
import {
  describeNotification,
  gatewayNotifications,
  newestCreatedAt,
  parseSeenAt,
  unreadCount,
  type AppNotification,
} from "./notifications";
import type { Session } from "./types";

const session: Session = { userId: "u1", email: "patient@demo.swiftmeds.test", accessToken: "token-1" };

const note = (template: string, params: Record<string, string> = {}, createdAt = "2026-11-19T08:00:00Z"): AppNotification => ({
  notificationId: `n-${template}-${createdAt}`,
  template,
  params,
  createdAt,
});

describe("describeNotification", () => {
  it("shows the pickup code and links to the hold when a hold is confirmed", () => {
    const view = describeNotification(note("TEMPLATE_HOLD_CONFIRMED", { hold_id: "h 1", pickup_code: "442BCX" }));
    expect(view.title).toBe("Hold confirmed");
    expect(view.body).toContain("442BCX");
    expect(view.href).toBe("/holds/h%201");
  });

  it("explains an expired and a rejected hold in plain words", () => {
    expect(describeNotification(note("TEMPLATE_HOLD_EXPIRED", { hold_id: "h1" }))).toMatchObject({ title: "Hold expired", href: "/holds/h1" });
    const rejected = describeNotification(note("TEMPLATE_HOLD_REJECTED", { hold_id: "h1", reason: "STOCK_NOT_AVAILABLE" }));
    expect(rejected.title).toBe("Hold not placed");
    expect(rejected.body).not.toContain("STOCK_NOT_AVAILABLE"); // no internal codes
  });

  it("names the medicine and pharmacy of a back-in-stock alert and links to the pharmacy", () => {
    const product = MOCK_PRODUCTS[0];
    const pharmacy = MOCK_PHARMACIES[0];
    const view = describeNotification(note("TEMPLATE_BACK_IN_STOCK", { product_id: product.productId, pharmacy_id: pharmacy.pharmacyId }));
    expect(view.title).toContain(product.brandName);
    expect(view.body).toContain(pharmacy.name);
    expect(view.href).toBe(`/pharmacy/${pharmacy.pharmacyId}`);
  });

  it("falls back to a general line for a template the app does not know", () => {
    expect(describeNotification(note("TEMPLATE_PHARMACY_VERIFIED"))).toMatchObject({ title: "Update from SwiftMed", href: "/holds" });
  });
});

describe("unread count", () => {
  const list = [note("A", {}, "2026-11-19T08:00:00Z"), note("B", {}, "2026-11-19T09:00:00Z"), note("C", {}, "2026-11-19T10:00:00Z")];

  it("counts everything before the bell was ever opened", () => {
    expect(unreadCount(list, undefined)).toBe(3);
  });

  it("counts only what is newer than the last seen one", () => {
    expect(unreadCount(list, "2026-11-19T09:00:00Z")).toBe(1);
    expect(unreadCount(list, "2026-11-19T10:00:00Z")).toBe(0);
  });

  it("remembers the newest creation time, whatever the order", () => {
    expect(newestCreatedAt([list[1], list[2], list[0]])).toBe("2026-11-19T10:00:00Z");
    expect(newestCreatedAt([])).toBeUndefined();
  });

  it("ignores a seen cookie that is not a date", () => {
    expect(parseSeenAt("not-a-date")).toBeUndefined();
    expect(parseSeenAt(undefined)).toBeUndefined();
    expect(parseSeenAt("2026-11-19T10:00:00Z")).toBe("2026-11-19T10:00:00Z");
  });
});

describe("gatewayNotifications", () => {
  it("asks the gateway with the patient's token and returns the newest first", async () => {
    const fetchFn = vi.fn(async () =>
      Response.json({
        notifications: [
          { notificationId: "old", template: "TEMPLATE_HOLD_EXPIRED", params: { hold_id: "h1" }, createdAt: "2026-11-18T08:00:00Z" },
          { notificationId: "", template: "TEMPLATE_HOLD_EXPIRED" }, // no ID: dropped
          { notificationId: "new", template: "TEMPLATE_HOLD_CONFIRMED", params: { pickup_code: "1", n: 5 }, createdAt: "2026-11-19T08:00:00Z" },
        ],
      }),
    );
    const list = await gatewayNotifications({ gatewayUrl: "http://gw.test", fetchFn }).list(session, 5);

    const [url, init] = fetchFn.mock.calls[0] as unknown as [URL, RequestInit];
    expect(String(url)).toBe("http://gw.test/api/v1/notifications?pageSize=5");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer token-1");
    expect(list.map((n) => n.notificationId)).toEqual(["new", "old"]);
    expect(list[0].params).toEqual({ pickup_code: "1" }); // non-string values dropped
  });

  it("reports an ended session as unauthorized", async () => {
    const fetchFn = vi.fn(async () => new Response(null, { status: 401 }));
    await expect(gatewayNotifications({ gatewayUrl: "http://gw.test", fetchFn }).list(session)).rejects.toMatchObject({ kind: "unauthorized" });
  });

  it("reports a gateway that cannot be reached", async () => {
    const fetchFn = vi.fn(async () => {
      throw new TypeError("fetch failed");
    });
    const error = await gatewayNotifications({ gatewayUrl: "http://gw.test", fetchFn }).list(session).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HoldError);
    expect((error as HoldError).kind).toBe("unreachable");
  });
});
