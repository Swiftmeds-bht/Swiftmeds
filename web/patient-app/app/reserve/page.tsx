import type { Metadata } from "next";
import Link from "next/link";

import { createHoldAction } from "@/app/actions";
import { ArrowLeftIcon, InfoIcon } from "@/components/Icons";
import { ReserveForm } from "@/components/ReserveForm";
import { requireSession } from "@/lib/auth";
import { formatPrice } from "@/lib/format";
import { HOLD_MINUTES } from "@/lib/demo-holds";
import { MAX_HOLD_QUANTITY, holdsBackend } from "@/lib/holds";

export const metadata: Metadata = { title: "Reserve" };

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";

// Opened from the Reserve link on a search result (components/ResultsList.tsx).
// The names and price in the link are only for display; the hold itself is
// checked by the server against real stock.
export default async function ReservePage({ searchParams }: { searchParams: SearchParamsIn }) {
  const sp = await searchParams;
  const productId = first(sp.product);
  const pharmacyId = first(sp.pharmacy);
  const returnTo = `/reserve?${new URLSearchParams(
    Object.entries(sp).flatMap(([k, v]) => (typeof v === "string" ? [[k, v]] : [])),
  )}`;
  await requireSession(returnTo);

  if (!productId || !pharmacyId) {
    return (
      <div className="page">
        <h1>Reserve</h1>
        <div className="problem" role="alert">
          <h2>Choose a pharmacy first</h2>
          <p>Search for your medicine and press Reserve next to a pharmacy.</p>
          <Link href="/" className="button">
            Search
          </Link>
        </div>
      </div>
    );
  }

  const available = Number.parseInt(first(sp.available), 10);
  const maxQuantity = Math.min(MAX_HOLD_QUANTITY, Number.isFinite(available) && available > 0 ? available : MAX_HOLD_QUANTITY);
  const price = first(sp.price);
  const demo = holdsBackend().source === "demo";

  return (
    <div className="page reserve-page">
      <Link href="/" className="back-link">
        <ArrowLeftIcon size={16} /> New search
      </Link>
      <h1>Reserve</h1>
      <div className="card reserve-card">
        <dl className="details">
          <div>
            <dt>Medicine</dt>
            <dd>{first(sp.name) || "Medicine"}</dd>
          </div>
          <div>
            <dt>Pharmacy</dt>
            <dd>{first(sp.at) || "Pharmacy"}</dd>
          </div>
          {price && (
            <div>
              <dt>Price each</dt>
              <dd>{formatPrice(price)}</dd>
            </div>
          )}
        </dl>
        <ReserveForm
          action={createHoldAction}
          pharmacyId={pharmacyId}
          productId={productId}
          maxQuantity={maxQuantity}
          returnTo={returnTo}
        />
      </div>
      <p className="notice" role="note">
        <InfoIcon />
        <span>
          The pharmacy checks its stock first. If the units are there you get a pickup code
          {demo ? ` and ${HOLD_MINUTES} minutes to collect` : " and a time to collect by"}. You pay at the counter.
        </span>
      </p>
    </div>
  );
}
