// Where the patient app gets its data (SCRUM-117). The real gateway is the
// default: demo data (lib/mock-data.ts, lib/demo-holds.ts) is used only when
// USE_DEMO_DATA=true, so a misconfigured server never shows demo stock as if it
// were live. Server-side only: read on each call, so a Docker image built once
// works in any environment.

/** The gateway on a developer machine, as started by `make up`. */
export const DEFAULT_GATEWAY_URL = "http://localhost:8080";

type Env = Readonly<Record<string, string | undefined>>;

/** True when USE_DEMO_DATA is "true" (any case, spaces ignored). */
export function demoDataEnabled(env: Env = process.env): boolean {
  return env.USE_DEMO_DATA?.trim().toLowerCase() === "true";
}

/**
 * The gateway base URL to call, or "" for demo mode. Every backend treats ""
 * as "use the demo", so this is the one place that decides.
 */
export function gatewayUrl(env: Env = process.env): string {
  if (demoDataEnabled(env)) return "";
  return env.GATEWAY_URL?.trim() || DEFAULT_GATEWAY_URL;
}
