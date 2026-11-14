import { describe, expect, it } from "vitest";

import { DashboardApiError } from "./api";
import { collectErrorMessage, loginErrorMessage } from "./messages";

describe("collectErrorMessage", () => {
  it("says clearly when the pickup code is wrong (Holds answers 403)", () => {
    const message = collectErrorMessage(new DashboardApiError("forbidden", 403));
    expect(message).toMatch(/wrong pickup code/i);
    expect(message).toMatch(/still active/i);
    expect(collectErrorMessage(new DashboardApiError("invalid collect request", 400))).toMatch(/wrong pickup code/i);
  });

  it("explains holds that can no longer be collected", () => {
    expect(collectErrorMessage(new DashboardApiError("x", 412))).toMatch(/already collected, cancelled or has expired/);
    expect(collectErrorMessage(new DashboardApiError("x", 409))).toMatch(/refresh/i);
    expect(collectErrorMessage(new DashboardApiError("x", 404))).toMatch(/not found/i);
  });

  it("falls back to the Gateway message or a generic one", () => {
    expect(collectErrorMessage(new DashboardApiError("The Gateway could not complete the request.", 503))).toBe(
      "The Gateway could not complete the request.",
    );
    expect(collectErrorMessage(new Error("boom"))).toMatch(/could not collect/i);
  });
});

describe("loginErrorMessage", () => {
  it("maps wrong credentials and a missing route", () => {
    expect(loginErrorMessage(new DashboardApiError("x", 401))).toBe("Email or password is incorrect.");
    expect(loginErrorMessage(new DashboardApiError("x", 404))).toMatch(/not available/);
    expect(loginErrorMessage(new Error("This account does not have pharmacy staff access."))).toMatch(/staff access/);
  });
});
