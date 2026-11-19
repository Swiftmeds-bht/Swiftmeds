import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { cancelHoldAction } from "@/app/actions";
import { AutoRefresh } from "@/components/AutoRefresh";
import { HoldCard } from "@/components/HoldCard";
import { requirePatient } from "@/lib/auth";
import { errorMessage, statusInfo } from "@/lib/hold-format";
import { HoldError, holdsBackend } from "@/lib/holds";
import { nameBook } from "@/lib/names";
import type { Hold } from "@/lib/types";

export const metadata: Metadata = { title: "My holds" };
export const dynamic = "force-dynamic";

export default async function HoldsPage() {
  const session = await requirePatient("/holds");

  let holds: Hold[] = [];
  let failed: HoldError["kind"] | undefined;
  try {
    holds = await holdsBackend().listHolds(session);
  } catch (err) {
    failed = err instanceof HoldError ? err.kind : "server";
  }
  if (failed === "unauthorized") redirect("/session-ended?next=%2Fholds");

  const book = await nameBook(holds);
  const current = holds.filter((h) => !statusInfo(h.status).final);
  const past = holds.filter((h) => statusInfo(h.status).final);

  return (
    <div className="page narrow holds-page">
      <h1>My holds</h1>
      <p className="muted">Signed in as {session.email}</p>

      {failed ? (
        <div className="problem" role="alert">
          <h2>We could not load your holds</h2>
          <p>{errorMessage(failed)}</p>
          <Link href="/holds" className="button">
            Try again
          </Link>
        </div>
      ) : holds.length === 0 ? (
        <div className="empty">
          <p>You have no holds yet.</p>
          <p>
            <Link href="/">Search for a medicine</Link> and press Reserve next to a pharmacy.
          </p>
        </div>
      ) : (
        <>
          <section aria-labelledby="current-heading" className="hold-section">
            <h2 id="current-heading" className="section-heading">
              Current <span className="section-count">{current.length}</span>
            </h2>
            {current.some((h) => h.status === "HOLD_STATUS_PENDING") && <AutoRefresh />}
            {current.length === 0 ? (
              <p className="muted">Nothing waiting for you to collect.</p>
            ) : (
              <ul className="hold-list">
                {current.map((h) => (
                  <li key={h.holdId}>
                    <HoldCard hold={h} cancelAction={cancelHoldAction} variant="row" book={book} />
                  </li>
                ))}
              </ul>
            )}
          </section>
          {past.length > 0 && (
            <section aria-labelledby="past-heading" className="hold-section">
              <h2 id="past-heading" className="section-heading">
                Past <span className="section-count">{past.length}</span>
              </h2>
              <ul className="hold-list">
                {past.map((h) => (
                  <li key={h.holdId}>
                    <HoldCard hold={h} cancelAction={cancelHoldAction} variant="row" book={book} />
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </div>
  );
}
