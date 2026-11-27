import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { setPriceAction, setStockAction } from "@/app/pharmacy-portal/actions";
import { MedicineCell } from "@/components/portal/MedicineCell";
import { Notices, queryText, type SearchParams } from "@/components/portal/Notices";
import { formatPrice } from "@/lib/format";
import { DashboardApiError, listPharmacyStock, listPrices, type StockLevel } from "@/lib/portal/api";
import { portalSession } from "@/lib/portal/data";
import { displayDate } from "@/lib/portal/format";
import { lookupMedicines, UNKNOWN_MEDICINE, type MedicineName } from "@/lib/portal/names";
import { chhertumToNu } from "@/lib/portal/prices";

export const metadata: Metadata = { title: "Stock" };
export const dynamic = "force-dynamic";

/** priceChhertum: undefined when the medicine has no price yet, or prices could not be loaded. */
type NamedStock = StockLevel & { medicine: MedicineName; priceChhertum?: number };

function StockTable({ stock, pricesLoaded }: { stock: NamedStock[]; pricesLoaded: boolean }) {
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
        <caption className="sr-only">Medicine stock: on hand, reserved for holds, available to patients, and price</caption>
        <thead>
          <tr>
            <th scope="col">Medicine</th>
            <th scope="col" className="num-col">On hand</th>
            <th scope="col" className="num-col">Reserved</th>
            <th scope="col" className="num-col">Available</th>
            <th scope="col">Last confirmed</th>
            <th scope="col">Set on hand</th>
            <th scope="col">Price per pack</th>
          </tr>
        </thead>
        <tbody>
          {stock.map((item) => {
            const reserved = item.quantityReserved ?? 0;
            const available = item.quantityAvailable ?? 0;
            const inputId = `quantity-${item.productId}`;
            const priceId = `price-${item.productId}`;
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
                <td className="row-action" data-label="Price">
                  <form action={setPriceAction} className="inline-form price-form">
                    <input type="hidden" name="productId" value={item.productId} />
                    <label className="sr-only" htmlFor={priceId}>
                      Price per pack of {item.medicine.name}, in ngultrum
                      {item.priceChhertum ? ` (now ${formatPrice(item.priceChhertum)})` : " (no price yet)"}
                    </label>
                    <span className="price-prefix" aria-hidden="true">
                      Nu.
                    </span>
                    <input
                      id={priceId}
                      name="priceNu"
                      inputMode="decimal"
                      pattern="[0-9]+([.][0-9]{1,2})?"
                      placeholder={pricesLoaded ? "No price" : "—"}
                      defaultValue={item.priceChhertum ? chhertumToNu(item.priceChhertum) : ""}
                      required
                    />
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
  let pricesLoaded = true;
  try {
    const levels = await listPharmacyStock(session);
    const ids = levels.map((s) => s.productId);
    // Prices are extra: if they cannot be loaded the stock still shows (SCRUM-154).
    const [names, prices] = await Promise.all([
      lookupMedicines(ids),
      listPrices(session, ids).catch(() => {
        pricesLoaded = false;
        return new Map<string, number>();
      }),
    ]);
    stock = levels
      .map((s) => ({ ...s, medicine: names.get(s.productId) ?? UNKNOWN_MEDICINE, priceChhertum: prices.get(s.productId) }))
      .sort((a, b) => a.medicine.name.localeCompare(b.medicine.name));
  } catch (cause) {
    error = cause instanceof DashboardApiError ? cause.message : "Could not load the stock. Please try again.";
  }

  return (
    <div className="page">
      <div className="page-heading page-heading-row">
        <div>
          <h1>Stock</h1>
          <p className="muted">Set the total on hand and the price per pack. Units reserved for holds are protected.</p>
        </div>
        <Link href="/pharmacy-portal/upload" className="button button-secondary">
          Upload CSV
        </Link>
      </div>
      <Notices error={queryText(params.error) ?? error} notice={queryText(params.notice)} />
      {!error && !pricesLoaded && (
        <p className="notice notice-warning" role="note">
          The prices could not be loaded just now. Stock still works; reload the page to see prices.
        </p>
      )}
      {error ? null : (
        <section className="panel stack-table" aria-label="Medicine stock">
          <StockTable stock={stock} pricesLoaded={pricesLoaded} />
        </section>
      )}
    </div>
  );
}
