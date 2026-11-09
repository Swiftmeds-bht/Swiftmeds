import Link from "next/link";

import type { FormState } from "@/app/actions";
import { formatClock, holdNames, statusInfo } from "@/lib/hold-format";
import type { Hold } from "@/lib/types";

import { AutoRefresh } from "./AutoRefresh";
import { CancelHoldForm } from "./CancelHoldForm";
import { LoaderIcon } from "./Icons";
import { Countdown } from "./Countdown";
import { PickupTicket } from "./PickupTicket";

interface Props {
  hold: Hold;
  cancelAction: (state: FormState, form: FormData) => Promise<FormState>;
  /** "detail": the hold page, with the pickup code large. "row": one entry in My holds. */
  variant?: "detail" | "row";
  /** The hold turned active moments ago: play the ticket's one-time settle. */
  justActivated?: boolean;
}

function StatusPill({ status }: { status: string }) {
  const info = statusInfo(status);
  return (
    <span className={`badge badge-${info.tone}`}>
      {/* Pending is live: a turning loader, not a decorative dot. */}
      {status === "HOLD_STATUS_PENDING" && <LoaderIcon size={14} />}
      {info.label}
    </span>
  );
}

function Message({ hold }: { hold: Hold }) {
  switch (hold.status) {
    case "HOLD_STATUS_PENDING":
      return <p className="hold-message">Checking with the pharmacy that the units are still there. This takes a few seconds.</p>;
    case "HOLD_STATUS_ACTIVE":
      return <p className="hold-message">Show this code at the counter. The pharmacy keeps the units for you until the time runs out.</p>;
    case "HOLD_STATUS_REJECTED":
      return (
        <p className="hold-message">
          The pharmacy could not hold it{hold.statusReason ? `: ${hold.statusReason}` : "."} Nothing was reserved.{" "}
          <Link href="/">Search again</Link> for another pharmacy.
        </p>
      );
    case "HOLD_STATUS_EXPIRED":
      return (
        <p className="hold-message">
          This hold ran out{hold.expiresAt ? ` at ${formatClock(hold.expiresAt)}` : ""} and the units went back on sale.{" "}
          <Link href="/">Search again</Link> to reserve it again.
        </p>
      );
    case "HOLD_STATUS_CANCELLED":
      return <p className="hold-message">You cancelled this hold. The units went back on sale.</p>;
    case "HOLD_STATUS_COLLECTED":
      return <p className="hold-message">You collected this medicine. Thank you.</p>;
    default:
      return null;
  }
}

export function HoldCard({ hold, cancelAction, variant = "detail", justActivated = false }: Props) {
  const names = holdNames(hold);
  const detail = variant === "detail";
  const active = hold.status === "HOLD_STATUS_ACTIVE";
  const quantity = hold.quantity ? `${hold.quantity} ${hold.quantity === 1 ? "unit" : "units"}` : "";
  // h1 on the hold page; h3 under the "Current" and "Past" headings of My holds.
  const Heading = detail ? "h1" : "h3";
  const selfHref = `/holds/${encodeURIComponent(hold.holdId)}`;

  return (
    <article className={`panel hold-card hold-${variant}`} aria-labelledby={`hold-${hold.holdId}`}>
      <div className="hold-head">
        <Heading id={`hold-${hold.holdId}`} className="hold-title">
          {detail ? names.medicine : <Link href={selfHref}>{names.medicine}</Link>}
        </Heading>
        <div className="hold-meta">
          {/* Live region: screen readers hear the new status when the page refreshes. */}
          <div role="status" aria-live="polite" className="hold-status">
            <StatusPill status={hold.status} />
          </div>
          <p className="muted">
            {quantity && `${quantity} · `}
            {names.pharmacy}
            {detail && names.address ? `, ${names.address}` : ""}
          </p>
        </div>
      </div>

      {detail && <Message hold={hold} />}

      {active && hold.pickupCode && (
        <PickupTicket code={hold.pickupCode} small={!detail} settle={detail && justActivated}>
          {hold.expiresAt && <Countdown expiresAt={hold.expiresAt} />}
        </PickupTicket>
      )}

      {detail && hold.status === "HOLD_STATUS_PENDING" && <AutoRefresh />}

      {active && (
        <CancelHoldForm
          action={cancelAction}
          holdId={hold.holdId}
          returnTo={detail ? selfHref : "/holds"}
          label={detail ? undefined : names.medicine}
        />
      )}
    </article>
  );
}
