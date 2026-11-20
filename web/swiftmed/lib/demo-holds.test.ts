import { beforeEach, describe, expect, it } from "vitest";

import { DEMO_ACCOUNT, DemoHolds, HOLD_MINUTES, PENDING_MS } from "./demo-holds";
import { HoldError } from "./holds-core";
import { MOCK_STOCK } from "./mock-data";
import type { Session } from "./types";

// Stock the demo can hold: plenty of units, and a pharmacy with only the last unit.
const plenty = MOCK_STOCK.find((s) => s.quantity >= 5)!;
const lastUnit = MOCK_STOCK.find((s) => s.quantity === 1)!;

let clock: number;
let demo: DemoHolds;
let me: Session;

const advance = (ms: number) => {
  clock += ms;
};

beforeEach(async () => {
  clock = Date.parse("2026-11-05T09:00:00Z");
  demo = new DemoHolds(() => clock);
  me = await demo.login(DEMO_ACCOUNT.email, DEMO_ACCOUNT.password);
});

async function kind(promise: Promise<unknown>): Promise<string> {
  try {
    await promise;
    return "no error";
  } catch (err) {
    return err instanceof HoldError ? err.kind : String(err);
  }
}

describe("accounts", () => {
  it("has a demo patient account", () => {
    expect(me.email).toBe(DEMO_ACCOUNT.email);
    expect(me.userId).toBeTruthy();
    expect(me.accessToken).toBeTruthy();
  });

  it("registers a new patient who can then sign in", async () => {
    await demo.register("Tashi@Example.bt", "correct-horse");
    const session = await demo.login("tashi@example.bt", "correct-horse");
    expect(session.email).toBe("tashi@example.bt");
  });

  it("refuses a taken email, a short password and a wrong password", async () => {
    expect(await kind(demo.register(DEMO_ACCOUNT.email, "another-password"))).toBe("email_taken");
    expect(await kind(demo.register("new@example.bt", "short"))).toBe("bad_request");
    expect(await kind(demo.register("not-an-email", "long-enough"))).toBe("bad_request");
    expect(await kind(demo.login(DEMO_ACCOUNT.email, "wrong-password"))).toBe("invalid_credentials");
    expect(await kind(demo.login("nobody@example.bt", "whatever-pass"))).toBe("invalid_credentials");
  });

  it("does not accept a made-up session", async () => {
    const forged = { ...me, accessToken: "forged" };
    expect(await kind(demo.listHolds(forged))).toBe("unauthorized");
  });
});

describe("holds", () => {
  it("is pending at first, then active with a pickup code and an expiry", async () => {
    const created = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 2 });
    expect(created.status).toBe("HOLD_STATUS_PENDING");
    expect(created.pickupCode).toBeUndefined();

    advance(PENDING_MS);
    const active = (await demo.getHold(me, created.holdId))!;
    expect(active.status).toBe("HOLD_STATUS_ACTIVE");
    expect(active.pickupCode).toMatch(/^\d{6}$/);
    expect(Date.parse(active.expiresAt!) - Date.parse(active.statusChangedAt!)).toBe(HOLD_MINUTES * 60_000);
  });

  it("expires when the time is up", async () => {
    const { holdId } = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 1 });
    advance(PENDING_MS + HOLD_MINUTES * 60_000);
    const hold = (await demo.getHold(me, holdId))!;
    expect(hold.status).toBe("HOLD_STATUS_EXPIRED");
    expect(hold.pickupCode).toBeUndefined();
  });

  it("is rejected when someone else took the last unit", async () => {
    const { holdId } = await demo.createHold(me, { pharmacyId: lastUnit.pharmacyId, productId: lastUnit.productId, quantity: 1 });
    advance(PENDING_MS);
    const hold = (await demo.getHold(me, holdId))!;
    expect(hold.status).toBe("HOLD_STATUS_REJECTED");
    expect(hold.statusReason).toBeTruthy();
  });

  it("is rejected when more units are asked for than are left", async () => {
    const tight = MOCK_STOCK.find((s) => s.quantity >= 2 && s.quantity < 5)!;
    const { holdId } = await demo.createHold(me, { pharmacyId: tight.pharmacyId, productId: tight.productId, quantity: tight.quantity + 1 });
    advance(PENDING_MS);
    expect((await demo.getHold(me, holdId))!.status).toBe("HOLD_STATUS_REJECTED");
  });

  it("checks the quantity and that the medicine is stocked there", async () => {
    const base = { pharmacyId: plenty.pharmacyId, productId: plenty.productId };
    expect(await kind(demo.createHold(me, { ...base, quantity: 0 }))).toBe("bad_request");
    expect(await kind(demo.createHold(me, { ...base, quantity: 6 }))).toBe("bad_request");
    expect(await kind(demo.createHold(me, { ...base, quantity: 1.5 }))).toBe("bad_request");
    expect(await kind(demo.createHold(me, { ...base, productId: "unknown", quantity: 1 }))).toBe("not_found");
  });

  it("can be cancelled while active, and only then", async () => {
    const { holdId } = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 1 });
    expect(await kind(demo.cancelHold(me, holdId))).toBe("not_active"); // still pending

    advance(PENDING_MS);
    const cancelled = await demo.cancelHold(me, holdId);
    expect(cancelled.status).toBe("HOLD_STATUS_CANCELLED");
    expect(cancelled.pickupCode).toBeUndefined();
    expect(await kind(demo.cancelHold(me, holdId))).toBe("not_active");
  });

  it("keeps each patient's holds private", async () => {
    const { holdId } = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 1 });
    await demo.register("other@example.bt", "other-password");
    const other = await demo.login("other@example.bt", "other-password");

    expect(await demo.getHold(other, holdId)).toBeUndefined();
    expect(await demo.listHolds(other)).toEqual([]);
    expect(await kind(demo.cancelHold(other, holdId))).toBe("not_found");
  });

  it("lists holds newest first", async () => {
    const first = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 1 });
    advance(1000);
    const second = await demo.createHold(me, { pharmacyId: plenty.pharmacyId, productId: plenty.productId, quantity: 1 });
    expect((await demo.listHolds(me)).map((h) => h.holdId)).toEqual([second.holdId, first.holdId]);
  });

  it("counts active holds against the stock", async () => {
    const tight = MOCK_STOCK.find((s) => s.quantity >= 2 && s.quantity <= 5)!;
    const key = { pharmacyId: tight.pharmacyId, productId: tight.productId };
    const first = await demo.createHold(me, { ...key, quantity: tight.quantity });
    advance(PENDING_MS);
    expect((await demo.getHold(me, first.holdId))!.status).toBe("HOLD_STATUS_ACTIVE");

    const second = await demo.createHold(me, { ...key, quantity: 1 });
    advance(PENDING_MS);
    expect((await demo.getHold(me, second.holdId))!.status).toBe("HOLD_STATUS_REJECTED");
  });
});
