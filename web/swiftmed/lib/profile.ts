// The signed-in patient's own profile (SCRUM-137): GET and PUT /api/v1/me/profile
// through the gateway (docs/api/openapi.yaml, getMyProfile and updateMyProfile).
// With USE_DEMO_DATA=true an in-memory store answers instead. Server-side only.
import { gatewayUrl as configuredGatewayUrl } from "./data-source";
import { errorForStatus } from "./holds";
import { HoldError } from "./holds-core";
import type { Session } from "./types";

export interface Profile {
  displayName: string;
  phone: string;
}

export const MAX_DISPLAY_NAME = 80;

// The gateway's rule (services/gateway/internal/app/profile.go): 8 to 15 digits,
// optional "+", after removing the separators people type.
const PHONE = /^\+?[1-9][0-9]{7,14}$/;
const normalisePhone = (phone: string) => phone.trim().replace(/[\s\-.()]/g, "");

/** The profile to save, or a sentence saying what to fix. Same rules as the gateway. */
export function checkProfile(input: Profile): { profile: Profile } | { error: string } {
  const displayName = input.displayName.trim();
  const phone = normalisePhone(input.phone);
  if (!displayName) return { error: "Enter your name." };
  if ([...displayName].length > MAX_DISPLAY_NAME) return { error: `Use at most ${MAX_DISPLAY_NAME} characters for your name.` };
  if (!PHONE.test(phone)) return { error: "Enter a phone number with 8 to 15 digits, for example 17123456 or +97517123456." };
  return { profile: { displayName, phone } };
}

/** First letter of the name, or of the email, for the account button. */
export function initialOf(name: string | undefined, email: string): string {
  const source = name?.trim() || email.trim();
  return ([...source][0] ?? "?").toLocaleUpperCase();
}

export interface ProfileBackend {
  readonly source: "gateway" | "demo";
  /** The saved profile, or undefined before the first save. */
  get(session: Session): Promise<Profile | undefined>;
  save(session: Session, profile: Profile): Promise<Profile>;
}

export function profileBackend(gatewayUrl = configuredGatewayUrl()): ProfileBackend {
  return gatewayUrl ? gatewayProfile({ gatewayUrl }) : demoProfile();
}

export interface GatewayProfileOptions {
  gatewayUrl: string;
  fetchFn?: typeof fetch;
  timeoutMs?: number;
}

const asProfile = (body: unknown): Profile => {
  const p = body as { displayName?: unknown; phone?: unknown } | null;
  return {
    displayName: typeof p?.displayName === "string" ? p.displayName : "",
    phone: typeof p?.phone === "string" ? p.phone : "",
  };
};

export function gatewayProfile(options: GatewayProfileOptions): ProfileBackend {
  const fetchFn = options.fetchFn ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? 4000;
  const url = new URL("/api/v1/me/profile", options.gatewayUrl);

  async function send(session: Session, init: RequestInit): Promise<Response> {
    try {
      return await fetchFn(url, {
        ...init,
        headers: { Authorization: `Bearer ${session.accessToken}`, ...(init.body ? { "Content-Type": "application/json" } : {}) },
        cache: "no-store",
        signal: AbortSignal.timeout(timeoutMs),
      });
    } catch (cause) {
      const timedOut = (cause as { name?: unknown } | null)?.name === "TimeoutError";
      throw new HoldError(timedOut ? "timeout" : "unreachable", "Could not reach the gateway", { cause });
    }
  }

  return {
    source: "gateway",
    async get(session) {
      const res = await send(session, { method: "GET" });
      if (res.status === 404) return undefined; // not saved yet
      if (!res.ok) throw errorForStatus(res.status, "hold");
      return asProfile(await res.json().catch(() => null));
    },
    async save(session, profile) {
      const res = await send(session, { method: "PUT", body: JSON.stringify(profile) });
      if (res.status === 400) {
        // The gateway's sentence says what to fix; errorMessage shows it as is.
        const detail = ((await res.json().catch(() => null)) as { error?: unknown } | null)?.error;
        const sentence = typeof detail === "string" && detail ? `${detail[0].toUpperCase()}${detail.slice(1)}.` : "Check your name and phone number.";
        throw new HoldError("bad_request", sentence);
      }
      if (!res.ok) throw errorForStatus(res.status, "hold");
      return asProfile(await res.json().catch(() => null));
    },
  };
}

const demoProfiles = new Map<string, Profile>();

/** In-memory profiles for USE_DEMO_DATA=true, kept until the server restarts. */
export function demoProfile(): ProfileBackend {
  return {
    source: "demo",
    get: async (session) => demoProfiles.get(session.userId),
    async save(session, input) {
      const checked = checkProfile(input);
      if ("error" in checked) throw new HoldError("bad_request", checked.error);
      demoProfiles.set(session.userId, checked.profile);
      return checked.profile;
    },
  };
}
