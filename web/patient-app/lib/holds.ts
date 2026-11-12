// Accounts and holds for the patient app. With GATEWAY_URL set, every call goes
// to the REST gateway (docs/api/openapi.yaml). Without it, a clearly labelled
// in-memory demo answers instead (lib/demo-holds.ts). Server-side only: the
// access token never reaches the browser.
import { demoBackend } from "./demo-holds";
import { HoldError, type HoldsBackend } from "./holds-core";
import { tokenFields } from "./refresh";
import type { Hold } from "./types";

export { HoldError, MAX_HOLD_QUANTITY, type HoldErrorKind, type HoldsBackend } from "./holds-core";

/** The backend for this server: the gateway when GATEWAY_URL is set, else the demo. */
export function holdsBackend(gatewayUrl = process.env.GATEWAY_URL ?? ""): HoldsBackend {
  return gatewayUrl ? gatewayBackend({ gatewayUrl }) : demoBackend();
}

// ---------------------------------------------------------------------------
// Gateway

export interface GatewayOptions {
  gatewayUrl: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

export function errorForStatus(status: number, action: "login" | "register" | "hold"): HoldError {
  switch (status) {
    case 400:
      return new HoldError("bad_request", `The gateway rejected the request (HTTP ${status})`);
    case 401:
      return action === "login"
        ? new HoldError("invalid_credentials", "Wrong email or password")
        : new HoldError("unauthorized", "Not signed in, or the session ended");
    case 403:
      return new HoldError("unauthorized", "This account cannot do that");
    case 404:
      return action === "hold"
        ? new HoldError("not_found", "No such hold")
        : new HoldError("not_available", "The gateway has no such route yet");
    case 409:
      return action === "register"
        ? new HoldError("email_taken", "The email is already registered")
        : new HoldError("not_active", "The hold changed; reload it");
    case 412:
      return new HoldError("not_active", "The hold is no longer active");
    case 501:
      return new HoldError("not_available", "The gateway has no such route yet");
    case 408:
    case 504:
      return new HoldError("timeout", `The gateway timed out (HTTP ${status})`);
    default:
      return new HoldError("server", `The gateway failed (HTTP ${status})`);
  }
}

export function gatewayBackend(options: GatewayOptions): HoldsBackend {
  const fetchFn = options.fetchFn ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? 5000;

  async function call(
    method: "GET" | "POST",
    path: string,
    action: "login" | "register" | "hold",
    { body, token }: { body?: unknown; token?: string } = {},
  ): Promise<unknown> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (token) headers.Authorization = `Bearer ${token}`;

    let res: Response;
    try {
      res = await fetchFn(new URL(path, options.gatewayUrl).toString(), {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        cache: "no-store",
        signal: AbortSignal.timeout(timeoutMs),
      });
    } catch (cause) {
      // By name, not instanceof: a DOMException from another realm is not this realm's Error.
      const name = (cause as { name?: unknown } | null)?.name;
      const timedOut = name === "TimeoutError" || name === "AbortError";
      throw new HoldError(timedOut ? "timeout" : "unreachable", timedOut ? "The gateway did not answer in time" : "Could not reach the gateway", { cause });
    }
    if (!res.ok) throw errorForStatus(res.status, action);
    if (res.status === 204) return undefined;
    try {
      return await res.json();
    } catch (cause) {
      throw new HoldError("server", "The gateway sent an answer that is not JSON", { cause });
    }
  }

  const asHold = (body: unknown): Hold => {
    const hold = body as Partial<Hold> | null;
    if (typeof hold?.holdId !== "string" || typeof hold.status !== "string") {
      throw new HoldError("server", "The gateway sent a hold without an ID or status");
    }
    return hold as Hold;
  };

  return {
    source: "gateway",

    async register(email, password) {
      await call("POST", "/api/v1/auth/register", "register", { body: { email, password, role: "ROLE_PATIENT" } });
    },

    async login(email, password) {
      const body = (await call("POST", "/api/v1/auth/login", "login", { body: { email, password } })) as {
        userId?: string;
        tokens?: Parameters<typeof tokenFields>[0];
      } | null;
      const tokens = tokenFields(body?.tokens);
      if (typeof body?.userId !== "string" || !body.userId || !tokens) {
        throw new HoldError("server", "The gateway sent a login answer without a user or token");
      }
      return { userId: body.userId, email, ...tokens };
    },

    async createHold(session, input) {
      return asHold(await call("POST", "/api/v1/holds", "hold", { body: input, token: session.accessToken }));
    },

    async getHold(session, holdId) {
      try {
        return asHold(await call("GET", `/api/v1/holds/${encodeURIComponent(holdId)}`, "hold", { token: session.accessToken }));
      } catch (err) {
        if (err instanceof HoldError && err.kind === "not_found") return undefined;
        throw err;
      }
    },

    async listHolds(session) {
      const body = (await call("GET", "/api/v1/holds?pageSize=50", "hold", { token: session.accessToken })) as {
        holds?: unknown[];
      } | null;
      return (body?.holds ?? []).map(asHold);
    },

    async cancelHold(session, holdId) {
      return asHold(
        await call("POST", `/api/v1/holds/${encodeURIComponent(holdId)}/cancel`, "hold", {
          body: { reason: "Cancelled by the patient" },
          token: session.accessToken,
        }),
      );
    },
  };
}
