import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { unwatchAction } from "@/app/actions";
import { UnwatchForm } from "@/components/UnwatchForm";
import { requirePatient } from "@/lib/auth";
import { formatClock, errorMessage, holdNames } from "@/lib/hold-format";
import { HoldError } from "@/lib/holds";
import { nameBook } from "@/lib/names";
import type { WatchSubscription } from "@/lib/types";
import { watchlistBackend } from "@/lib/watchlist";

export const metadata: Metadata = { title: "Watchlist" };
export const dynamic = "force-dynamic";

export default async function WatchlistPage() {
  const session = await requirePatient("/watchlist");

  let subs: WatchSubscription[] = [];
  let failed: HoldError["kind"] | undefined;
  try {
    subs = await watchlistBackend().list(session);
  } catch (err) {
    failed = err instanceof HoldError ? err.kind : "server";
  }
  if (failed === "unauthorized") redirect("/session-ended?next=%2Fwatchlist");
  const book = await nameBook(subs);

  return (
    <div className="page narrow watchlist-page">
      <h1>Watchlist</h1>
      <p className="muted">
        Medicines you asked us to watch. When a pharmacy nearby gets one, an alert appears on any SwiftMed page.
      </p>

      {failed ? (
        <div className="problem" role="alert">
          <h2>We could not load your watchlist</h2>
          <p>{errorMessage(failed)}</p>
          <Link href="/watchlist" className="button">
            Try again
          </Link>
        </div>
      ) : subs.length === 0 ? (
        <div className="empty">
          <p>You are not watching any medicine.</p>
          <p>
            <Link href="/">Search for a medicine</Link>. If no pharmacy nearby has it, press Notify me when it is back.
          </p>
        </div>
      ) : (
        <ul className="watch-list">
          {subs.map((s) => {
            const { medicine } = holdNames({ productId: s.productId, pharmacyId: "" }, book);
            const back = Boolean(s.notifiedAt);
            return (
              <li key={s.subscriptionId} className="watch-row">
                <div>
                  <p className="watch-name">{medicine}</p>
                  <p className={back ? "badge badge-high" : "badge badge-neutral"}>{back ? "Back in stock" : "Waiting"}</p>
                  <p className="muted watch-meta">
                    {back ? `Found at ${formatClock(s.notifiedAt)}` : `Watching since ${formatClock(s.createdAt)}`}
                  </p>
                </div>
                <div className="watch-side">
                  {back && (
                    <Link href={`/results?q=${encodeURIComponent(medicine)}`} className="button button-small">
                      Search now<span className="sr-only"> for {medicine}</span>
                    </Link>
                  )}
                  <UnwatchForm action={unwatchAction} subscriptionId={s.subscriptionId} medicine={medicine} />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
