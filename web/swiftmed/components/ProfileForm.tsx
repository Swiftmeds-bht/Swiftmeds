"use client";

import { useActionState } from "react";

import { saveProfileAction, type ProfileState } from "@/app/actions";
import { MAX_DISPLAY_NAME } from "@/lib/profile";

export interface ProfileFormProps {
  displayName: string;
  phone: string;
  /** For tests: what to call instead of the server action. */
  action?: (prev: ProfileState, form: FormData) => Promise<ProfileState>;
}

/** Name and phone (SCRUM-137). The email is the sign-in and is not edited here. */
export function ProfileForm({ displayName, phone, action = saveProfileAction }: ProfileFormProps) {
  const [state, formAction, pending] = useActionState(action, { displayName, phone });

  return (
    <form action={formAction} className="panel profile-form" noValidate>
      <div className="form-field">
        <label htmlFor="displayName">Name</label>
        <input
          id="displayName"
          name="displayName"
          autoComplete="name"
          maxLength={MAX_DISPLAY_NAME}
          defaultValue={state.displayName}
          key={`name-${state.displayName}`}
          required
          aria-describedby={state.error ? "profile-error" : undefined}
        />
      </div>
      <div className="form-field">
        <label htmlFor="phone">Phone</label>
        <input
          id="phone"
          name="phone"
          type="tel"
          inputMode="tel"
          autoComplete="tel"
          defaultValue={state.phone}
          key={`phone-${state.phone}`}
          required
          aria-describedby={state.error ? "profile-error phone-hint" : "phone-hint"}
        />
        <p id="phone-hint" className="field-hint">
          So the pharmacy can reach you about a hold. For example 17123456.
        </p>
      </div>

      {state.error ? (
        <p id="profile-error" className="form-error" role="alert">
          {state.error}
        </p>
      ) : null}
      {state.saved && !pending ? (
        <p className="form-success" role="status">
          Profile saved.
        </p>
      ) : null}

      <div className="form-actions">
        <button type="submit" className="button" disabled={pending}>
          {pending ? "Saving…" : "Save profile"}
        </button>
      </div>
    </form>
  );
}
