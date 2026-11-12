"use server";

// Form actions for accounts and holds. They run on the server, so the access
// token stays in the httpOnly cookie. Each returns { error } for the form to
// show, or redirects on success. redirect() throws, so it is never inside try.
import { redirect } from "next/navigation";

import { clearSession, getSession, saveSession, withFreshToken } from "@/lib/auth";
import { errorMessage } from "@/lib/hold-format";
import { HoldError, MAX_HOLD_QUANTITY, holdsBackend } from "@/lib/holds";
import { safeNext } from "@/lib/session";
import type { Session } from "@/lib/types";
import { watchlistBackend } from "@/lib/watchlist";

export interface FormState {
  error?: string;
  /** Typed back into the form after an error, since React resets forms after an action. */
  email?: string;
}

const text = (form: FormData, name: string) => {
  const value = form.get(name);
  return typeof value === "string" ? value.trim() : "";
};

function failure(err: unknown): FormState {
  if (err instanceof HoldError) return { error: errorMessage(err.kind, err.message) };
  console.error(err);
  return { error: errorMessage("server") };
}

export async function loginAction(_prev: FormState, form: FormData): Promise<FormState> {
  const email = text(form, "email");
  const password = form.get("password");
  if (!email || typeof password !== "string" || !password) return { error: "Enter your email and password.", email };
  try {
    await saveSession(await holdsBackend().login(email, password));
  } catch (err) {
    return { ...failure(err), email };
  }
  redirect(safeNext(text(form, "next")));
}

export async function registerAction(_prev: FormState, form: FormData): Promise<FormState> {
  const email = text(form, "email");
  const password = form.get("password");
  const confirm = form.get("confirm");
  if (!email || typeof password !== "string" || !password) return { error: "Enter an email and a password.", email };
  if (password.length < 8) return { error: "Use a password of at least 8 characters.", email };
  if (password !== confirm) return { error: "The two passwords are not the same.", email };
  try {
    const backend = holdsBackend();
    await backend.register(email, password);
    await saveSession(await backend.login(email, password));
  } catch (err) {
    return { ...failure(err), email };
  }
  redirect(safeNext(text(form, "next")));
}

export async function logoutAction(): Promise<void> {
  await clearSession();
  redirect("/");
}

/** The session, or (after clearing a stale one) a redirect to sign in. */
async function sessionOrSignIn(returnTo: string): Promise<Session> {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(safeNext(returnTo, "/"))}`);
  return session;
}

async function signInAgain(returnTo: string): Promise<never> {
  await clearSession();
  redirect(`/login?next=${encodeURIComponent(safeNext(returnTo, "/"))}&expired=1`);
}

export async function createHoldAction(_prev: FormState, form: FormData): Promise<FormState> {
  const returnTo = text(form, "returnTo");
  const session = await sessionOrSignIn(returnTo);
  const quantity = Number(text(form, "quantity"));
  if (!Number.isInteger(quantity) || quantity < 1 || quantity > MAX_HOLD_QUANTITY) {
    return { error: `Choose 1 to ${MAX_HOLD_QUANTITY} units.` };
  }

  let holdId: string;
  try {
    const input = { pharmacyId: text(form, "pharmacyId"), productId: text(form, "productId"), quantity };
    const hold = await withFreshToken(session, (s) => holdsBackend().createHold(s, input));
    holdId = hold.holdId;
  } catch (err) {
    if (err instanceof HoldError && err.kind === "unauthorized") return signInAgain(returnTo);
    return failure(err);
  }
  redirect(`/holds/${encodeURIComponent(holdId)}`);
}

export interface WatchState extends FormState {
  /** Set once the medicine is on the watchlist. */
  watching?: boolean;
}

/** "Notify me when it is back": watches one medicine around the searched place. */
export async function watchAction(_prev: WatchState, form: FormData): Promise<WatchState> {
  const returnTo = text(form, "returnTo");
  const session = await sessionOrSignIn(returnTo);
  const latitude = Number(text(form, "lat"));
  const longitude = Number(text(form, "lng"));
  const radiusM = Number(text(form, "radius"));
  const productId = text(form, "productId");
  if (!productId || !Number.isFinite(latitude) || !Number.isFinite(longitude) || !(radiusM > 0)) {
    return { error: "Search again, then press Notify me." };
  }
  try {
    await withFreshToken(session, (s) => watchlistBackend().subscribe(s, { productId, location: { latitude, longitude }, radiusM }));
  } catch (err) {
    if (err instanceof HoldError && err.kind === "unauthorized") return signInAgain(returnTo);
    return failure(err);
  }
  return { watching: true };
}

export async function unwatchAction(_prev: FormState, form: FormData): Promise<FormState> {
  const session = await sessionOrSignIn("/watchlist");
  try {
    const subscriptionId = text(form, "subscriptionId");
    await withFreshToken(session, (s) => watchlistBackend().unsubscribe(s, subscriptionId));
  } catch (err) {
    if (err instanceof HoldError && err.kind === "unauthorized") return signInAgain("/watchlist");
    // Already gone is what the patient wanted.
    if (!(err instanceof HoldError && err.kind === "not_found")) return failure(err);
  }
  redirect("/watchlist");
}

export async function cancelHoldAction(_prev: FormState, form: FormData): Promise<FormState> {
  const holdId = text(form, "holdId");
  const returnTo = safeNext(text(form, "returnTo"), `/holds/${encodeURIComponent(holdId)}`);
  const session = await sessionOrSignIn(returnTo);
  try {
    await withFreshToken(session, (s) => holdsBackend().cancelHold(s, holdId));
  } catch (err) {
    if (err instanceof HoldError && err.kind === "unauthorized") return signInAgain(returnTo);
    return failure(err);
  }
  redirect(returnTo);
}
