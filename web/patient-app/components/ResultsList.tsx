import Link from "next/link";

import { formatDistance, formatPrice, formatQuantity, freshnessLabel } from "@/lib/format";
import type { ProductResult } from "@/lib/types";

function ProductCard({ product }: { product: ProductResult }) {
  const headingId = `product-${product.productId}`;
  return (
    <section className="card" aria-labelledby={headingId}>
      <div className="card-head">
        <h3 id={headingId}>
          {product.brandName} <span className="strength">{product.strength}</span>
        </h3>
        <p className="generic">{product.genericName}</p>
      </div>
      {product.offers.length === 0 ? (
        <p className="empty">Not in stock at any pharmacy in this area.</p>
      ) : (
        <ol className="offers" aria-label={`Pharmacies with ${product.brandName}, best match first`}>
          {product.offers.map((o, index) => {
            const fresh = freshnessLabel(o.freshnessScore);
            return (
              <li key={o.pharmacyId} className="offer">
                <div className="offer-main">
                  {index === 0 && product.offers.length > 1 && <p className="best-match">Best match</p>}
                  <Link href={`/pharmacy/${o.pharmacyId}`} className="offer-name">
                    {o.pharmacyName}
                  </Link>
                  <p className="offer-meta">
                    {formatDistance(o.distanceM)} away · {o.address}
                  </p>
                  <p className={`badge badge-${fresh.level}`}>{fresh.text}</p>
                </div>
                <dl className="offer-facts">
                  <dt className="sr-only">Price</dt>
                  <dd className="offer-price">{formatPrice(o.priceChhertum)}</dd>
                  <dt className="sr-only">In stock</dt>
                  <dd className="offer-qty">{formatQuantity(o.quantityAvailable)} in stock</dd>
                </dl>
              </li>
            );
          })}
        </ol>
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
      <p role="status" className="results-summary">
        {total === 0
          ? `No pharmacy nearby has “${query}” in stock.`
          : `${total} ${total === 1 ? "pharmacy has" : "pharmacies have"} “${query}” in stock.`}
      </p>
      {total > 0 && (
        <p className="hint">Best match first, by distance, how recently stock was confirmed, and pharmacy reliability.</p>
      )}
      {results.map((r) => (
        <ProductCard key={r.productId} product={r} />
      ))}
      {substitutes.length > 0 && (
        <section aria-labelledby="substitutes-heading" className="substitutes">
          <div>
            <h2 id="substitutes-heading">Same medicine, other brands</h2>
            <p className="muted">Same active ingredient and strength. Ask the pharmacist before switching.</p>
          </div>
          {substitutes.map((r) => (
            <ProductCard key={r.productId} product={r} />
          ))}
        </section>
      )}
    </div>
  );
}
