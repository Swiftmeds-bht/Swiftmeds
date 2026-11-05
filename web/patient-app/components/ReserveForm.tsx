"use client";

import { useActionState, useId } from "react";

import type { FormState } from "@/app/actions";

interface Props {
  action: (state: FormState, form: FormData) => Promise<FormState>;
  pharmacyId: string;
  productId: string;
  /** The most units the patient may pick (1..5, and no more than are in stock). */
  maxQuantity: number;
  /** This page, to come back to after signing in again. */
  returnTo: string;
}

export function ReserveForm({ action, pharmacyId, productId, maxQuantity, returnTo }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  const id = useId();
  const max = Math.max(1, maxQuantity);

  return (
    <form action={formAction} className="reserve-form">
      <input type="hidden" name="pharmacyId" value={pharmacyId} />
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="returnTo" value={returnTo} />
      <div className="form-field">
        <label htmlFor={`${id}-quantity`}>How many?</label>
        <select id={`${id}-quantity`} name="quantity" defaultValue="1">
          {Array.from({ length: max }, (_, i) => i + 1).map((n) => (
            <option key={n} value={n}>
              {n} {n === 1 ? "unit" : "units"}
            </option>
          ))}
        </select>
      </div>
      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}
      <button type="submit" className="button button-block" disabled={pending}>
        {pending ? "Reserving…" : "Reserve"}
      </button>
    </form>
  );
}
