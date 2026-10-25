import Link from "next/link";

import { formatDistance, formatPrice, formatQuantity, freshnessLabel } from "@/lib/format";
import type { ProductResult } from "@/lib/types";

function ProductCard({ product }: { product: ProductResult }) {
  const headingId = `product-${product.productId}`;
  return (
    <section className="card" aria-labelledby={headingId}>
      <h3 id={headingId}>
        {product.brandName} <span className="muted">{product.strength}</span>
      </h3>
      <p className="muted">{product.genericName}</p>
      {product.offers.length === 0 ? (
        <p className="empty">Not in stock at any pharmacy in this area.</p>
      ) : (
        <ul className="offers" aria-label={`Pharmacies with ${product.brandName}`}>
          {product.offers.map((o) => {
            const fresh = freshnessLabel(o.freshnessScore);
            return (
              <li key={o.pharmacyId} className="offer">
                <div>
                  <Link href={`/pharmacy/${o.pharmacyId}`} className="offer-name">
                    {o.pharmacyName}
                  </Link>
                  <p className="muted">
                    {formatDistance(o.distanceM)} away · {o.address}
                  </p>
                  <p className={`badge badge-${fresh.level}`}>{fresh.text}</p>
                </div>
                <dl className="offer-facts">
                  <div>
                    <dt>Price</dt>
                    <dd>{formatPrice(o.priceChhertum)}</dd>
                  </div>
                  <div>
                    <dt>In stock</dt>
                    <dd>{formatQuantity(o.quantityAvailable)}</dd>
                  </div>
                </dl>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

interface Props {
  query: string;
  results: ProductResult[];
  substitutes: ProductResult[];
}

export function ResultsList({ query, results, substitutes }: Props) {
  if (results.length === 0) {
    return (
      <p className="empty" role="status">
        No medicines match “{query}”. Check the spelling or try the generic name.
      </p>
    );
  }
  const total = results.reduce((n, r) => n + r.offers.length, 0);
  return (
    <div className="results">
      <p role="status">
        {total === 0
          ? `No pharmacy nearby has “${query}” in stock.`
          : `${total} ${total === 1 ? "pharmacy has" : "pharmacies have"} “${query}” in stock.`}
      </p>
      {results.map((r) => (
        <ProductCard key={r.productId} product={r} />
      ))}
      {substitutes.length > 0 && (
        <section aria-labelledby="substitutes-heading" className="substitutes">
          <h2 id="substitutes-heading">Same medicine, other brands</h2>
          <p className="muted">Same active ingredient and strength. Ask the pharmacist before switching.</p>
          {substitutes.map((r) => (
            <ProductCard key={r.productId} product={r} />
          ))}
        </section>
      )}
    </div>
  );
}
