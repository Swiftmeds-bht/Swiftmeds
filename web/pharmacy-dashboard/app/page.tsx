import Link from "next/link";
import { redirect } from "next/navigation";

import { collectHoldAction, logoutAction, setStockAction } from "@/app/actions";
import {
  DashboardApiError,
  getUnmetDemand,
  listActiveHolds,
  listPharmacyStock,
  type Hold,
  type StockLevel,
  type UnmetDemandItem,
} from "@/lib/api";
import { getSession } from "@/lib/session";

type PageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

type LoadResult<T> = {
  data: T | null;
  error: string | null;
};

function queryText(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function errorText(error: unknown): string {
  if (error instanceof DashboardApiError) return error.message;
  return "Could not load this section. Please try again.";
}

async function load<T>(request: Promise<T>): Promise<LoadResult<T>> {
  try {
    return { data: await request, error: null };
  } catch (error) {
    return { data: null, error: errorText(error) };
  }
}

function displayDate(value?: string): string {
  if (!value) return "—";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";

  return new Intl.DateTimeFormat("en-GB", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Asia/Thimphu",
  }).format(date);
}

function StockTable({ stock }: { stock: StockLevel[] }) {
  if (stock.length === 0) {
    return <p className="empty-state">No stock entries are available for this pharmacy.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Product ID</th>
            <th scope="col">On hand</th>
            <th scope="col">Reserved</th>
            <th scope="col">Available</th>
            <th scope="col">Last confirmed</th>
            <th scope="col">Update quantity</th>
          </tr>
        </thead>
        <tbody>
          {stock.map((item) => {
            const reserved = item.quantityReserved ?? 0;
            const inputId = `quantity-${item.productId}`;

            return (
              <tr key={item.productId}>
                <td>
                  <code className="id-value">{item.productId}</code>
                </td>
                <td>{item.quantityOnHand ?? 0}</td>
                <td>{reserved}</td>
                <td>
                  <strong>{item.quantityAvailable ?? 0}</strong>
                </td>
                <td>{displayDate(item.confirmedAt)}</td>
                <td>
                  <form action={setStockAction} className="inline-form">
                    <input type="hidden" name="productId" value={item.productId} />
                    <label className="sr-only" htmlFor={inputId}>
                      New quantity on hand for product {item.productId}
                    </label>
                    <input
                      id={inputId}
                      name="quantityOnHand"
                      type="number"
                      min={reserved}
                      step="1"
                      defaultValue={item.quantityOnHand ?? 0}
                      required
                    />
                    <button className="button button-small" type="submit">
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

function HoldsTable({ holds }: { holds: Hold[] }) {
  if (holds.length === 0) {
    return <p className="empty-state">There are no active holds for this pharmacy.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Hold ID</th>
            <th scope="col">Product ID</th>
            <th scope="col">Quantity</th>
            <th scope="col">Expires</th>
            <th scope="col">Collect hold</th>
          </tr>
        </thead>
        <tbody>
          {holds.map((hold) => {
            const inputId = `pickup-code-${hold.holdId}`;

            return (
              <tr key={hold.holdId}>
                <td>
                  <code className="id-value">{hold.holdId}</code>
                </td>
                <td>
                  <code className="id-value">{hold.productId ?? "—"}</code>
                </td>
                <td>{hold.quantity ?? "—"}</td>
                <td>{displayDate(hold.expiresAt)}</td>
                <td>
                  <form action={collectHoldAction} className="inline-form">
                    <input type="hidden" name="holdId" value={hold.holdId} />
                    <label className="sr-only" htmlFor={inputId}>
                      Pickup code for hold {hold.holdId}
                    </label>
                    <input
                      id={inputId}
                      name="pickupCode"
                      type="text"
                      inputMode="numeric"
                      autoComplete="off"
                      placeholder="Enter code"
                      required
                    />
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

function UnmetDemandTable({ items }: { items: UnmetDemandItem[] }) {
  if (items.length === 0) {
    return <p className="empty-state">No unmet demand was reported for your area in the last 7 days.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Medicine</th>
            <th scope="col">Searches</th>
            <th scope="col">Unmet</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.productId}>
              <td>{item.productName}</td>
              <td>{item.searchCount ?? 0}</td>
              <td><strong>{item.unmetCount ?? 0}</strong></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default async function DashboardPage({ searchParams }: PageProps) {
  const session = await getSession();
  if (!session) redirect("/login");

  const [params, stockResult, holdsResult, demandResult] = await Promise.all([searchParams,
    load(listPharmacyStock(session)),
    load(listActiveHolds(session)),
    load(getUnmetDemand(session)),
  ]);

  const error = queryText(params.error);
  const notice = queryText(params.notice);
  const stock = stockResult.data ?? [];
  const unmetDemand = demandResult.data ?? [];
  const holds = holdsResult.data ?? [];

  return (
    <main className="dashboard-shell">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">SwiftMeds · Pharmacy</p>
          <h1>Pharmacy dashboard</h1>
          <p className="dashboard-subtitle">
            Update medicine availability and manage patient pickups.
          </p>
        </div>

        <div className="header-actions">
          <Link className="button button-primary" href="/upload">
            Upload stock CSV
          </Link>
          <form action={logoutAction}>
            <button className="button button-secondary" type="submit">
              Sign out
            </button>
          </form>
        </div>
      </header>

      {error ? (
        <p className="message message-error" role="alert">
          {error}
        </p>
      ) : null}

      {notice ? (
        <p className="message message-success" role="status">
          {notice}
        </p>
      ) : null}

      <section className="summary-grid" aria-label="Pharmacy summary">
        <article className="summary-card">
          <span className="summary-label">Stock records</span>
          <strong className="summary-value">{stockResult.data === null ? "—" : stock.length}</strong>
        </article>
        <article className="summary-card">
          <span className="summary-label">Active holds</span>
          <strong className="summary-value">{holdsResult.data === null ? "—" : holds.length}</strong>
        </article>
        <article className="summary-card summary-card-wide">
          <span className="summary-label">Pharmacy ID</span>
          <code className="id-value">{session.pharmacyId}</code>
        </article>
      </section>

      <section className="panel" aria-labelledby="unmet-demand-heading">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Area trends</p>
            <h2 id="unmet-demand-heading">Most requested unavailable medicines</h2>
          </div>
          <p className="panel-hint">Top 10 for your pharmacy’s area · last 7 days</p>
        </div>

        {demandResult.error ? (
          <p className="message message-error" role="alert">
            {demandResult.error}
          </p>
        ) : (
          <UnmetDemandTable items={unmetDemand} />
        )}
      </section>


      <section className="panel" aria-labelledby="stock-heading">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Inventory</p>
            <h2 id="stock-heading">Medicine stock</h2>
          </div>
          <p className="panel-hint">Set the total on-hand quantity. Reserved units are protected.</p>
        </div>

        {stockResult.error ? (
          <p className="message message-error" role="alert">
            {stockResult.error}
          </p>
        ) : (
          <StockTable stock={stock} />
        )}
      </section>

      <section className="panel" id="holds" aria-labelledby="holds-heading">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Pickup management</p>
            <h2 id="holds-heading">Active holds</h2>
          </div>
          <p className="panel-hint">Enter the code provided by the patient to complete a pickup.</p>
        </div>

        {holdsResult.error ? (
          <p className="message message-error" role="alert">
            {holdsResult.error}
          </p>
        ) : (
          <HoldsTable holds={holds} />
        )}
      </section>

      <footer className="dashboard-footer">
        Pickup codes are entered at the counter and are never shown in this dashboard.
      </footer>
    </main>
  );
}