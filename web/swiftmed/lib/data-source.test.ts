import { describe, expect, it } from "vitest";

import { DEFAULT_GATEWAY_URL, demoDataEnabled, gatewayUrl } from "./data-source";

describe("gatewayUrl", () => {
  it("uses the real gateway by default", () => {
    expect(gatewayUrl({})).toBe(DEFAULT_GATEWAY_URL);
    expect(gatewayUrl({ GATEWAY_URL: "" })).toBe(DEFAULT_GATEWAY_URL);
  });

  it("uses GATEWAY_URL when set", () => {
    expect(gatewayUrl({ GATEWAY_URL: " http://gateway:8080 " })).toBe("http://gateway:8080");
  });

  it.each(["true", "TRUE", " true "])("switches to demo data only for USE_DEMO_DATA=%j", (value) => {
    expect(gatewayUrl({ USE_DEMO_DATA: value, GATEWAY_URL: "http://gateway:8080" })).toBe("");
    expect(demoDataEnabled({ USE_DEMO_DATA: value })).toBe(true);
  });

  it.each(["", "false", "1", "yes", undefined])("keeps the gateway for USE_DEMO_DATA=%j", (value) => {
    expect(gatewayUrl({ USE_DEMO_DATA: value })).toBe(DEFAULT_GATEWAY_URL);
    expect(demoDataEnabled({ USE_DEMO_DATA: value })).toBe(false);
  });
});
