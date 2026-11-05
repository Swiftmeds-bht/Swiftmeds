"use client";

import Link from "next/link";
import { useActionState, useId } from "react";

import type { FormState } from "@/app/actions";

interface Props {
  mode: "login" | "register";
  action: (state: FormState, form: FormData) => Promise<FormState>;
  /** Where to go after signing in (a same-site path). */
  next?: string;
}

// Sign in and create account. A plain form posting to a server action, so it
// also works before JavaScript loads; the error comes back as form state.
export function AuthForm({ mode, action, next = "" }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  const id = useId();
  const errorId = `${id}-error`;
  const login = mode === "login";
  const otherHref = `${login ? "/register" : "/login"}${next ? `?next=${encodeURIComponent(next)}` : ""}`;

  return (
    <form action={formAction} className="auth-form" aria-describedby={state.error ? errorId : undefined} noValidate={false}>
      <input type="hidden" name="next" value={next} />
      <div className="form-field">
        <label htmlFor={`${id}-email`}>Email</label>
        <input id={`${id}-email`} name="email" type="email" autoComplete="email" required defaultValue={state.email} />
      </div>
      <div className="form-field">
        <label htmlFor={`${id}-password`}>Password</label>
        <input
          id={`${id}-password`}
          name="password"
          type="password"
          autoComplete={login ? "current-password" : "new-password"}
          minLength={login ? undefined : 8}
          maxLength={72}
          required
          aria-describedby={login ? undefined : `${id}-password-hint`}
        />
        {!login && (
          <p id={`${id}-password-hint`} className="hint">
            At least 8 characters.
          </p>
        )}
      </div>
      {!login && (
        <div className="form-field">
          <label htmlFor={`${id}-confirm`}>Repeat password</label>
          <input id={`${id}-confirm`} name="confirm" type="password" autoComplete="new-password" minLength={8} maxLength={72} required />
        </div>
      )}
      {state.error && (
        <p id={errorId} className="form-error" role="alert">
          {state.error}
        </p>
      )}
      <button type="submit" className="button button-block" disabled={pending}>
        {pending ? (login ? "Signing in…" : "Creating account…") : login ? "Sign in" : "Create account"}
      </button>
      <p className="auth-switch">
        {login ? "New to SwiftMed? " : "Already have an account? "}
        <Link href={otherHref}>{login ? "Create an account" : "Sign in"}</Link>
      </p>
    </form>
  );
}
