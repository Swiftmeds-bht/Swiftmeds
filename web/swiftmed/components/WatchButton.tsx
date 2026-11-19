"use client";

import Link from "next/link";
import { useActionState } from "react";

import type { WatchState } from "@/app/actions";
import type { GeoPoint } from "@/lib/types";

interface Props {
  action: (state: WatchState, form: FormData) => Promise<WatchState>;
  productId: string;
  /** e.g. "Demo Paracetamol 500 500mg", for screen readers. */
  medicine: string;
  point: GeoPoint;
  radiusM: number;
  /** This search, to come back to after signing in. */
  returnTo: string;
}

// Shown on a medicine nobody nearby has: one press puts it on the watchlist,
// and an alert appears when a pharmacy in this area gets it (SCRUM-71).
export function WatchButton({ action, productId, medicine, point, radiusM, returnTo }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  if (state.watching) {
    return (
      <p className="watch-done" role="status">
        We will tell you when {medicine} is back nearby. <Link href="/watchlist">See your watchlist</Link>
      </p>
    );
  }
  return (
    <form action={formAction} className="watch-form">
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="lat" value={point.latitude} />
      <input type="hidden" name="lng" value={point.longitude} />
      <input type="hidden" name="radius" value={radiusM} />
      <input type="hidden" name="returnTo" value={returnTo} />
      <button type="submit" className="button button-small button-outline" disabled={pending}>
        {pending ? "Adding…" : "Notify me when it is back"}
        <span className="sr-only"> for {medicine}</span>
      </button>
      {state.error && (
        <p className="form-error" role="alert">
          {state.error}
        </p>
      )}
    </form>
  );
}
