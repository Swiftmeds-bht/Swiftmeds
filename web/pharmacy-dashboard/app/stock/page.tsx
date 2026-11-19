import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { setStockAction } from "@/app/actions";
import { MedicineCell } from "@/components/MedicineCell";
import { Notices, queryText, type SearchParams } from "@/components/Notices";
import { DashboardApiError, listPharmacyStock, type StockLevel } from "@/lib/api";
import { currentSession } from "@/lib/data";
import { displayDate } from "@/lib/format";
import { lookupMedicines, UNKNOWN_MEDICINE, type MedicineName } from "@/lib/names";

export const metadata: Metadata = { title: "Stock" };
export const dynamic = "force-dynamic";

type NamedStock = StockLevel & { medicine: MedicineName };

function StockTable({ stock }: { stock: NamedStock[] }) {
  if (stock.length === 0) {
    return (
      <p className="empty-state">
        No medicines are listed yet. <Link href="/upload">Upload a stock CSV</Link> to add them.
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
                <td>
                  <MedicineCell medicine={item.medicine} productId={item.productId} />
                </td>
                <td className="num-col num">{item.quantityOnHand ?? 0}</td>
                <td className="num-col num">{reserved}</td>
                <td className="num-col num">
                  {available > 0 ? <strong>{available}</strong> : <span className="badge badge-low">Out</span>}
                </td>
                <td className="cell-detail-only">{displayDate(item.confirmedAt)}</td>
                <td>
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
  const session = await currentSession();
  if (!session) redirect("/login");

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
        <Link href="/upload" className="button button-secondary">
          Upload CSV
        </Link>
      </div>
      <Notices error={queryText(params.error) ?? error} notice={queryText(params.notice)} />
      {error ? null : (
        <section className="panel" aria-label="Medicine stock">
          <StockTable stock={stock} />
        </section>
      )}
    </div>
  );
}
