// @vitest-environment node
import { describe, expect, it } from "vitest";

import { siteUrl } from "./site";

describe("siteUrl (SCRUM-157)", () => {
  it("prefers SITE_URL, then Vercel's production address, then the deployment's, then localhost", () => {
    expect(siteUrl({ SITE_URL: "https://swiftmed.example/", VERCEL_PROJECT_PRODUCTION_URL: "a.vercel.app" })).toBe("https://swiftmed.example");
    expect(siteUrl({ VERCEL_PROJECT_PRODUCTION_URL: "swiftmeds-nine.vercel.app", VERCEL_URL: "x-123.vercel.app" })).toBe(
      "https://swiftmeds-nine.vercel.app",
    );
    expect(siteUrl({ VERCEL_URL: "x-123.vercel.app" })).toBe("https://x-123.vercel.app");
    expect(siteUrl({})).toBe("http://localhost:3000");
  });

  it("keeps an explicit scheme and ignores junk", () => {
    expect(siteUrl({ SITE_URL: "http://localhost:9000" })).toBe("http://localhost:9000");
    expect(siteUrl({ SITE_URL: "  " })).toBe("http://localhost:3000");
    expect(siteUrl({ SITE_URL: "https://" })).toBe("http://localhost:3000");
  });
});
