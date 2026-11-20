import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { collectHoldAction } from "@/app/pharmacy-portal/actions";
import { MedicineCell } from "@/components/portal/MedicineCell";
import { Notices, queryText, type SearchParams } from "@/components/portal/Notices";
import { DashboardApiError } from "@/lib/portal/api";
import { activeHolds, portalSession, nameHolds, type NamedHold } from "@/lib/portal/data";
import { displayDate } from "@/lib/portal/format";

export const metadata: Metadata = { title: "Holds" };
export const dynamic = "force-dynamic";

function HoldsTable({ holds }: { holds: NamedHold[] }) {
  if (holds.length === 0) return <p className="empty-state">No one is waiting for a pickup right now.</p>;
  return (
    <div className="table-wrap">
      <table>
        <caption className="sr-only">Holds waiting for pickup, soonest to expire first</caption>
        <thead>
          <tr>
            <th scope="col">Medicine</th>
            <th scope="col" className="num-col">Units</th>
            <th scope="col">Collect by</th>
            <th scope="col">Pickup code</th>
          </tr>
        </thead>
        <tbody>
          {holds.map((h) => {
            const inputId = `pickup-code-${h.holdId}`;
            return (
              <tr key={h.holdId} id={`hold-${h.holdId}`}>
                <td className="row-main">
                  <MedicineCell medicine={h.medicine} productId={h.productId} />
                </td>
                <td className="num-col num" data-label="Units">
                  {h.quantity ?? "—"}
                </td>
                <td>
                  <span className={`pickup-time${h.timeLeft === "Expired" ? " is-expired" : ""}`}>{h.timeLeft}</span>
                  <span className="cell-detail">{displayDate(h.expiresAt)}</span>
                </td>
                <td className="row-action">
                  <form action={collectHoldAction} className="inline-form">
                    <input type="hidden" name="holdId" value={h.holdId} />
                    <label className="sr-only" htmlFor={inputId}>
                      Pickup code for {h.medicine.name}, {h.quantity ?? 0} units
                    </label>
                    <input id={inputId} name="pickupCode" type="text" autoComplete="off" autoCapitalize="characters" placeholder="Code" required />
                    <button className="button button-small" type="submit">
                      Collect
                    </button>
                  </form>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/** Holds waiting at the counter (SCRUM-138): enter the patient's code to hand over. */
export default async function HoldsPage({ searchParams }: { searchParams: SearchParams }) {
  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  const params = await searchParams;
  let holds: NamedHold[] = [];
  let error: string | undefined;
  try {
    holds = await nameHolds(await activeHolds(session));
  } catch (cause) {
    error = cause instanceof DashboardApiError ? cause.message : "Could not load the holds. Please try again.";
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1>Holds</h1>
        <p className="muted">Ask the patient for their pickup code, enter it and hand over the medicine.</p>
      </div>
      <Notices error={queryText(params.error) ?? error} notice={queryText(params.notice)} />
      {error ? null : (
        <section className="panel stack-table" aria-label="Holds waiting for pickup">
          <HoldsTable holds={holds} />
        </section>
      )}
    </div>
  );
}
