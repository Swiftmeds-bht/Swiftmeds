"use client";

import { useActionState, useId, useState } from "react";

import type { FormState } from "@/app/actions";
import { formatPrice } from "@/lib/format";

interface Props {
  action: (state: FormState, form: FormData) => Promise<FormState>;
  pharmacyId: string;
  productId: string;
  /** The most units the patient may pick (1..5, and no more than are in stock). */
  maxQuantity: number;
  /** This page, to come back to after signing in again. */
  returnTo: string;
  /** Price of one unit in chhertum, when the pharmacy listed one: shows the total to pay at the counter. */
  unitPriceChhertum?: string;
}

export function ReserveForm({ action, pharmacyId, productId, maxQuantity, returnTo, unitPriceChhertum }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  const [quantity, setQuantity] = useState(1);
  const id = useId();
  const max = Math.max(1, maxQuantity);
  const unit = Number(unitPriceChhertum);
  const hasPrice = Number.isFinite(unit) && unit > 0;

  return (
    <form action={formAction} className="reserve-form">
      <input type="hidden" name="pharmacyId" value={pharmacyId} />
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="returnTo" value={returnTo} />
      <div className="form-field">
        <label htmlFor={`${id}-quantity`}>How many?</label>
        <select
          id={`${id}-quantity`}
          name="quantity"
          defaultValue="1"
          onChange={(event) => setQuantity(Number(event.target.value) || 1)}
        >
          {Array.from({ length: max }, (_, i) => i + 1).map((n) => (
            <option key={n} value={n}>
              {n} {n === 1 ? "unit" : "units"}
            </option>
          ))}
        </select>
      </div>
      {hasPrice && (
        <p className="reserve-total" aria-live="polite">
          <span>To pay at the counter</span>
          <strong className="num">{formatPrice(unit * quantity)}</strong>
        </p>
      )}
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
