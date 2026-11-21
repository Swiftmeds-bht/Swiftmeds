"use client";

import { useActionState, useId } from "react";

import type { ReviewState } from "@/app/admin/actions";

interface Props {
  action: (state: ReviewState, form: FormData) => Promise<ReviewState>;
  pharmacyId: string;
  name: string;
  /** The tab the admin is on, to come back to it. */
  tab: string;
  /** What can be done now: a pending pharmacy is approved or rejected, an approved one suspended. */
  allowed: "review" | "suspend";
}

/**
 * The admin's buttons on one pharmacy (SCRUM-142). Rejecting and suspending
 * ask for a reason the pharmacy will see; they sit behind a disclosure so the
 * main action, Approve, stays one click. Plain forms: they work before
 * JavaScript loads.
 */
export function ReviewActions({ action, pharmacyId, name, tab, allowed }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  const id = useId();
  const hidden = (
    <>
      <input type="hidden" name="pharmacyId" value={pharmacyId} />
      <input type="hidden" name="name" value={name} />
      <input type="hidden" name="tab" value={tab} />
    </>
  );
  const reasonForm = (decision: "reject" | "suspend", label: string, button: string) => (
    <details className="review-more">
      <summary>{label}</summary>
      <form action={formAction} className="review-reason">
        {hidden}
        <input type="hidden" name="decision" value={decision} />
        <label htmlFor={`${id}-${decision}`}>Reason (the pharmacy sees it)</label>
        <textarea id={`${id}-${decision}`} name="reason" rows={3} maxLength={500} required />
        <button type="submit" className="button button-small button-outline button-danger" disabled={pending}>
          {button}
        </button>
      </form>
    </details>
  );

  return (
    <div className="review-actions">
      {allowed === "review" ? (
        <>
          <form action={formAction}>
            {hidden}
            <input type="hidden" name="decision" value="approve" />
            <button type="submit" className="button button-small" disabled={pending} aria-label={`Approve ${name}`}>
              Approve
            </button>
          </form>
          {reasonForm("reject", "Reject…", "Reject pharmacy")}
        </>
      ) : (
        reasonForm("suspend", "Suspend…", "Suspend pharmacy")
      )}
      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}
    </div>
  );
}
