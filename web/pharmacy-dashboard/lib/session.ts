// Server-side session helpers for the pharmacist dashboard.
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export const SESSION_COOKIE = "swiftmed_pharmacy_session";
export const PHARMACY_STAFF_ROLE = "ROLE_PHARMACY_STAFF" as const;

export interface DashboardSession {
  userId: string;
  role: typeof PHARMACY_STAFF_ROLE;
  pharmacyId: string;
  accessToken: string;
  refreshToken: string;
  expiresAt?: string;
}

interface LoginResponse {
  userId?: unknown;
  role?: unknown;
  tokens?: {
    accessToken?: unknown;
    refreshToken?: unknown;
    accessTokenExpiresAt?: unknown;
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/**
 * Reads pharmacy_id from the Auth JWT so the dashboard can form the stock URL.
 * This does not verify the token; the API gateway must verify it and enforce
 * the staff role and pharmacy permissions on every request.
 */
function pharmacyIDFromAccessToken(accessToken: string): string {
  const payloadPart = accessToken.split(".")[1];
  if (!payloadPart) {
    throw new Error("Auth returned an invalid access token.");
  }

  let payload: unknown;
  try {
    payload = JSON.parse(Buffer.from(payloadPart, "base64url").toString("utf8"));
  } catch {
    throw new Error("Auth returned an invalid access token.");
  }

  if (!isRecord(payload) || typeof payload.pharmacy_id !== "string" || !payload.pharmacy_id) {
    throw new Error("This pharmacy staff account is not linked to a pharmacy.");
  }

  return payload.pharmacy_id;
}

export function sessionFromLogin(response: LoginResponse): DashboardSession {
  if (typeof response.userId !== "string" || !response.userId) {
    throw new Error("Auth returned no user ID.");
  }

  if (response.role !== PHARMACY_STAFF_ROLE) {
    throw new Error("This account does not have pharmacy staff access.");
  }

  const accessToken = response.tokens?.accessToken;
  const refreshToken = response.tokens?.refreshToken;
  const expiresAt = response.tokens?.accessTokenExpiresAt;

  if (typeof accessToken !== "string" || !accessToken) {
    throw new Error("Auth returned no access token.");
  }
  if (typeof refreshToken !== "string" || !refreshToken) {
    throw new Error("Auth returned no refresh token.");
  }
  if (expiresAt !== undefined && typeof expiresAt !== "string") {
    throw new Error("Auth returned an invalid access token expiry.");
  }

  if (typeof expiresAt === "string") {
    const expiry = Date.parse(expiresAt);
    if (!Number.isFinite(expiry) || expiry <= Date.now()) {
      throw new Error("Auth returned an expired access token.");
    }
  }

  return {
    userId: response.userId,
    role: PHARMACY_STAFF_ROLE,
    pharmacyId: pharmacyIDFromAccessToken(accessToken),
    accessToken,
    refreshToken,
    ...(typeof expiresAt === "string" ? { expiresAt } : {}),
  };
}

export async function getSession(): Promise<DashboardSession | undefined> {
  const value = (await cookies()).get(SESSION_COOKIE)?.value;
  if (!value) return undefined;

  try {
    const parsed: unknown = JSON.parse(Buffer.from(value, "base64url").toString("utf8"));
    if (!isRecord(parsed)) return undefined;

    const { userId, role, pharmacyId, accessToken, refreshToken, expiresAt } = parsed;

    if (
      typeof userId !== "string" ||
      !userId ||
      role !== PHARMACY_STAFF_ROLE ||
      typeof pharmacyId !== "string" ||
      !pharmacyId ||
      typeof accessToken !== "string" ||
      !accessToken ||
      typeof refreshToken !== "string" ||
      !refreshToken
    ) {
      return undefined;
    }

    if (expiresAt !== undefined) {
      if (typeof expiresAt !== "string") return undefined;
      const expiry = Date.parse(expiresAt);
      if (!Number.isFinite(expiry) || expiry <= Date.now()) return undefined;
    }

    return {
      userId,
      role: PHARMACY_STAFF_ROLE,
      pharmacyId,
      accessToken,
      refreshToken,
      ...(typeof expiresAt === "string" ? { expiresAt } : {}),
    };
  } catch {
    return undefined;
  }
}

export async function saveSession(session: DashboardSession): Promise<void> {
  const maxAge = session.expiresAt
    ? Math.max(0, Math.floor((Date.parse(session.expiresAt) - Date.now()) / 1000))
    : 15 * 60;

  (await cookies()).set(SESSION_COOKIE, Buffer.from(JSON.stringify(session)).toString("base64url"), {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    maxAge,
  });
}

export async function clearSession(): Promise<void> {
  (await cookies()).delete(SESSION_COOKIE);
}

export async function requireSession(): Promise<DashboardSession> {
  const session = await getSession();
  if (!session) redirect("/login");
  return session;
}