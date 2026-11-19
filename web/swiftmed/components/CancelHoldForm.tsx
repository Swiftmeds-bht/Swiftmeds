"use client";

import { useActionState } from "react";

import type { FormState } from "@/app/actions";

interface Props {
  action: (state: FormState, form: FormData) => Promise<FormState>;
  holdId: string;
  /** Where to show the result: the hold page or the list. */
  returnTo: string;
  /** Names the hold for screen readers when several Cancel buttons are on one page. */
  label?: string;
}

export function CancelHoldForm({ action, holdId, returnTo, label }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  return (
    <form action={formAction} className="cancel-form">
      <input type="hidden" name="holdId" value={holdId} />
      <input type="hidden" name="returnTo" value={returnTo} />
      <button type="submit" className="button button-outline button-danger" disabled={pending}>
        {pending ? "Cancelling…" : "Cancel hold"}
        {label && <span className="sr-only"> for {label}</span>}
      </button>
      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}
    </form>
  );
}
