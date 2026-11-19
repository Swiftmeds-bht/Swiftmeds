import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { setStockAction } from "@/app/pharmacy-portal/actions";
import { MedicineCell } from "@/components/portal/MedicineCell";
import { Notices, queryText, type SearchParams } from "@/components/portal/Notices";
import { DashboardApiError, listPharmacyStock, type StockLevel } from "@/lib/portal/api";
import { portalSession } from "@/lib/portal/data";
import { displayDate } from "@/lib/portal/format";
import { lookupMedicines, UNKNOWN_MEDICINE, type MedicineName } from "@/lib/portal/names";

export const metadata: Metadata = { title: "Stock" };
export const dynamic = "force-dynamic";

type NamedStock = StockLevel & { medicine: MedicineName };

function StockTable({ stock }: { stock: NamedStock[] }) {
  if (stock.length === 0) {
    return (
      <p className="empty-state">
        No medicines are listed yet. <Link href="/pharmacy-portal/upload">Upload a stock CSV</Link> to add them.
      </p>
    );
  }
  return (
    <div className="table-wrap">
      <table>
        <caption className="sr-only">Medicine stock: on hand, reserved for holds, and available to patients</caption>
        <thead>
          <tr>
            <th scope="col">Medicine</th>
            <th scope="col" className="num-col">On hand</th>
            <th scope="col" className="num-col">Reserved</th>
            <th scope="col" className="num-col">Available</th>
            <th scope="col">Last confirmed</th>
            <th scope="col">Set on hand</th>
          </tr>
        </thead>
        <tbody>
          {stock.map((item) => {
            const reserved = item.quantityReserved ?? 0;
            const available = item.quantityAvailable ?? 0;
            const inputId = `quantity-${item.productId}`;
            return (
              <tr key={item.productId}>
                <td className="row-main">
                  <MedicineCell medicine={item.medicine} productId={item.productId} />
                </td>
                <td className="num-col num" data-label="On hand">
                  {item.quantityOnHand ?? 0}
                </td>
                <td className="num-col num" data-label="Reserved">
                  {reserved}
                </td>
                <td className="num-col num" data-label="Available">
                  {available > 0 ? <strong>{available}</strong> : <span className="badge badge-low">Out</span>}
                </td>
                <td className="cell-detail-only" data-label="Confirmed">
                  {displayDate(item.confirmedAt)}
                </td>
                <td className="row-action">
                  <form action={setStockAction} className="inline-form">
                    <input type="hidden" name="productId" value={item.productId} />
                    <label className="sr-only" htmlFor={inputId}>
                      New quantity on hand for {item.medicine.name}
                    </label>
                    <input id={inputId} name="quantityOnHand" type="number" min={reserved} step="1" defaultValue={item.quantityOnHand ?? 0} required />
                    <button className="button button-small button-secondary" type="submit">
                      Save
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

/** Stock (SCRUM-138): every medicine this pharmacy lists, by name. */
export default async function StockPage({ searchParams }: { searchParams: SearchParams }) {
  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  const params = await searchParams;
  let stock: NamedStock[] = [];
  let error: string | undefined;
  try {
    const levels = await listPharmacyStock(session);
    const names = await lookupMedicines(levels.map((s) => s.productId));
    stock = levels
      .map((s) => ({ ...s, medicine: names.get(s.productId) ?? UNKNOWN_MEDICINE }))
      .sort((a, b) => a.medicine.name.localeCompare(b.medicine.name));
  } catch (cause) {
    error = cause instanceof DashboardApiError ? cause.message : "Could not load the stock. Please try again.";
  }

  return (
    <div className="page">
      <div className="page-heading page-heading-row">
        <div>
          <h1>Stock</h1>
          <p className="muted">Set the total on hand. Units reserved for holds are protected.</p>
        </div>
        <Link href="/pharmacy-portal/upload" className="button button-secondary">
          Upload CSV
        </Link>
      </div>
      <Notices error={queryText(params.error) ?? error} notice={queryText(params.notice)} />
      {error ? null : (
        <section className="panel stack-table" aria-label="Medicine stock">
          <StockTable stock={stock} />
        </section>
      )}
    </div>
  );
}
