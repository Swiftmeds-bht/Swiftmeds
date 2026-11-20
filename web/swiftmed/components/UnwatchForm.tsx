"use client";

import { useActionState } from "react";

import type { FormState } from "@/app/actions";

interface Props {
  action: (state: FormState, form: FormData) => Promise<FormState>;
  subscriptionId: string;
  /** Names the row for screen readers, since every row has a Remove button. */
  medicine: string;
}

export function UnwatchForm({ action, subscriptionId, medicine }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  return (
    <form action={formAction} className="cancel-form">
      <input type="hidden" name="subscriptionId" value={subscriptionId} />
      <button type="submit" className="button button-small button-outline" disabled={pending}>
        {pending ? "Removing…" : "Remove"}
        <span className="sr-only"> {medicine} from the watchlist</span>
      </button>
      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}
    </form>
  );
}
