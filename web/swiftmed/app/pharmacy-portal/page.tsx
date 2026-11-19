import Link from "next/link";
import { redirect } from "next/navigation";

import { MedicineCell } from "@/components/portal/MedicineCell";
import { Notices, queryText, type SearchParams } from "@/components/portal/Notices";
import { DashboardApiError, getUnmetDemand, listPharmacyStock, type UnmetDemandItem } from "@/lib/portal/api";
import { activeHolds, portalSession, nameHolds, pharmacyLabel, type NamedHold } from "@/lib/portal/data";

export const dynamic = "force-dynamic";

async function load<T>(request: Promise<T>): Promise<{ data: T | null; error: string | null }> {
  try {
    return { data: await request, error: null };
  } catch (error) {
    return { data: null, error: error instanceof DashboardApiError ? error.message : "Could not load this section. Please try again." };
  }
}

function NextPickups({ holds }: { holds: NamedHold[] }) {
  if (holds.length === 0) return <p className="empty-state">No one is waiting for a pickup right now.</p>;
  return (
    <ul className="pickup-list">
      {holds.slice(0, 5).map((h) => (
        <li key={h.holdId} className="pickup-row">
          <MedicineCell medicine={h.medicine} productId={h.productId} />
          <span className="pickup-qty num">
            {h.quantity ?? 0} {h.quantity === 1 ? "unit" : "units"}
          </span>
          <span className={`pickup-time${h.timeLeft === "Expired" ? " is-expired" : ""}`}>{h.timeLeft}</span>
          <Link href={`/pharmacy-portal/holds#hold-${h.holdId}`} className="button button-small button-secondary">
            Collect
          </Link>
        </li>
      ))}
    </ul>
  );
}

function UnmetDemandTable({ items }: { items: UnmetDemandItem[] }) {
  if (items.length === 0) return <p className="empty-state">No unmet demand was reported for your area in the last 7 days.</p>;
  return (
    <div className="table-wrap">
      <table>
        <caption className="sr-only">Medicines people searched for nearby but could not find</caption>
        <thead>
          <tr>
            <th scope="col">Medicine</th>
            <th scope="col" className="num-col">Searches</th>
            <th scope="col" className="num-col">Not found</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.productId}>
              <td className="medicine-name">{item.productName}</td>
              <td className="num-col num">{item.searchCount ?? 0}</td>
              <td className="num-col num">
                <strong>{item.unmetCount ?? 0}</strong>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Overview (SCRUM-138): what needs doing now, and what people nearby could not find. */
export default async function OverviewPage({ searchParams }: { searchParams: SearchParams }) {
  const session = await portalSession();
  if (!session) redirect("/login?next=%2Fpharmacy-portal");

  const [params, pharmacy, holds, stock, demand] = await Promise.all([
    searchParams,
    pharmacyLabel(session),
    load(activeHolds(session).then((h) => nameHolds(h))),
    load(listPharmacyStock(session)),
    load(getUnmetDemand(session)),
  ]);

  const inStock = stock.data?.filter((s) => (s.quantityAvailable ?? 0) > 0).length;
  const outOfStock = stock.data?.filter((s) => (s.quantityAvailable ?? 0) <= 0).length;
  const figure = (n: number | undefined) => (n === undefined ? "—" : n);

  return (
    <div className="page">
      <div className="page-heading">
        <h1>{pharmacy}</h1>
        <p className="muted">Today&apos;s pickups and stock at a glance.</p>
      </div>
      <Notices error={queryText(params.error)} notice={queryText(params.notice)} />

      <section className="stat-grid" aria-label="Summary">
        <Link href="/pharmacy-portal/holds" className="stat stat-link">
          <span className="stat-value num">{figure(holds.data?.length)}</span>
          <span className="stat-label">Waiting for pickup</span>
        </Link>
        <Link href="/pharmacy-portal/stock" className="stat stat-link">
          <span className="stat-value num">{figure(inStock)}</span>
          <span className="stat-label">Medicines in stock</span>
        </Link>
        <Link href="/pharmacy-portal/stock" className="stat stat-link">
          <span className="stat-value num">{figure(outOfStock)}</span>
          <span className="stat-label">Out of stock</span>
        </Link>
      </section>

      <section className="panel" aria-labelledby="pickups-heading">
        <div className="panel-heading">
          <h2 id="pickups-heading">Next pickups</h2>
          <Link href="/pharmacy-portal/holds" className="text-link">
            All holds
          </Link>
        </div>
        {holds.error ? (
          <p className="message message-error" role="alert">
            {holds.error}
          </p>
        ) : (
          <NextPickups holds={holds.data ?? []} />
        )}
      </section>

      <section className="panel" aria-labelledby="demand-heading">
        <div className="panel-heading">
          <h2 id="demand-heading">Asked for nearby, not found</h2>
          <p className="panel-hint">Top 10 in your area · last 7 days</p>
        </div>
        {demand.error ? (
          <p className="message message-error" role="alert">
            {demand.error}
          </p>
        ) : (
          <UnmetDemandTable items={demand.data ?? []} />
        )}
      </section>
    </div>
  );
}
