import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { cancelHoldAction } from "@/app/actions";
import { HoldCard } from "@/components/HoldCard";
import { ArrowLeftIcon } from "@/components/Icons";
import { requireSession } from "@/lib/auth";
import { errorMessage, justActivated } from "@/lib/hold-format";
import { HoldError, holdsBackend } from "@/lib/holds";
import type { Hold } from "@/lib/types";

export const metadata: Metadata = { title: "Your hold" };
// Always per request: the status changes while the patient watches.
export const dynamic = "force-dynamic";

type Params = Promise<{ id: string }>;

export default async function HoldPage({ params }: { params: Params }) {
  const { id } = await params;
  const self = `/holds/${encodeURIComponent(id)}`;
  const session = await requireSession(self);

  let hold: Hold | undefined;
  let failed: HoldError["kind"] | undefined;
  let settle = false;
  try {
    hold = await holdsBackend().getHold(session, id);
    settle = hold ? justActivated(hold) : false;
  } catch (err) {
    failed = err instanceof HoldError ? err.kind : "server";
  }
  if (failed === "unauthorized") redirect(`/session-ended?next=${encodeURIComponent(self)}`);

  return (
    <div className="page narrow hold-page">
      <Link href="/holds" className="back-link">
        <ArrowLeftIcon size={16} /> My holds
      </Link>
      {hold ? (
        <HoldCard hold={hold} cancelAction={cancelHoldAction} justActivated={settle} />
      ) : (
        <div className="problem" role="alert">
          <h2>{failed ? "We could not load this hold" : "We could not find this hold"}</h2>
          <p>{failed ? errorMessage(failed) : "It may belong to another account, or the link is wrong."}</p>
          {failed ? (
            <Link href={self} className="button">
              Try again
            </Link>
          ) : (
            <Link href="/holds" className="button">
              See my holds
            </Link>
          )}
        </div>
      )}
    </div>
  );
}
