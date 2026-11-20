import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  esbuild: { jsx: "automatic" },
  resolve: { alias: { "@": path.resolve(import.meta.dirname, ".") } },
  test: {
    environment: "jsdom",
    include: ["**/*.test.{ts,tsx}"],
    exclude: ["node_modules/**", ".next/**"],
    // Tests never need a running gateway: the default backends are the demo ones.
    // Gateway tests pass a gateway URL or set the env themselves.
    env: { USE_DEMO_DATA: "true" },
  },
});
