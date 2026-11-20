import Link from "next/link";

import { formatDistance, formatPrice, formatQuantity, freshnessLabel } from "@/lib/format";
import type { GeoPoint, PharmacyOffer, ProductResult } from "@/lib/types";

import { CheckIcon, MapPinIcon } from "./Icons";
import { WatchButton } from "./WatchButton";

/** The reserve page for one offer. Names and price go along for display only. */
export function reserveHref(product: ProductResult, offer: PharmacyOffer): string {
  const params = new URLSearchParams({
    product: product.productId,
    pharmacy: offer.pharmacyId,
    name: `${product.brandName} ${product.strength}`.trim(),
    at: offer.pharmacyName,
    available: String(offer.quantityAvailable),
  });
  if (offer.priceChhertum && offer.priceChhertum !== "0") params.set("price", offer.priceChhertum);
  return `/reserve?${params}`;
}

/** How recently stock was confirmed: three bars, filled by level, plus the words. */
function Freshness({ score }: { score: number | undefined }) {
  const fresh = freshnessLabel(score);
  return (
    <p className={`fresh fresh-${fresh.level}`}>
      <span className="fresh-bars" aria-hidden="true">
        <i />
        <i />
        <i />
      </span>
      {fresh.text}
    </p>
  );
}

/** What a "Notify me" button needs: the searched place, and the action to call. */
export interface WatchOptions {
  action: React.ComponentProps<typeof WatchButton>["action"];
  point: GeoPoint;
  radiusM: number;
  returnTo: string;
}

function ProductCard({ product, watch }: { product: ProductResult; watch?: WatchOptions }) {
  const headingId = `product-${product.productId}`;
  return (
    <section className="product" aria-labelledby={headingId}>
      <div className="product-head">
        <h3 id={headingId}>
          {product.brandName} <span className="strength">{product.strength}</span>
        </h3>
        <p className="generic">{product.genericName}</p>
      </div>
      {product.offers.length === 0 ? (
        <div className="empty-offers">
          <p className="empty">Not in stock at any pharmacy in this area.</p>
          {watch && (
            <WatchButton
              action={watch.action}
              productId={product.productId}
              medicine={`${product.brandName} ${product.strength}`.trim()}
              point={watch.point}
              radiusM={watch.radiusM}
              returnTo={watch.returnTo}
            />
          )}
        </div>
      ) : (
        <ol className="offers" aria-label={`Pharmacies with ${product.brandName}, best match first`}>
          {product.offers.map((o, index) => {
            const best = index === 0 && product.offers.length > 1;
            return (
              <li key={o.pharmacyId} className={best ? "offer offer-best" : "offer"}>
                <div className="offer-main">
                  {best && (
                    <p className="best-match">
                      <CheckIcon size={16} />
                      Best match
                    </p>
                  )}
                  <Link href={`/pharmacy/${o.pharmacyId}`} className="offer-name">
                    {o.pharmacyName}
                  </Link>
                  <p className="offer-meta">
                    <MapPinIcon size={15} />
                    <span>
                      <span className="offer-distance">{formatDistance(o.distanceM)}</span> away
                      {o.address && <> · {o.address}</>}
                    </span>
                  </p>
                  <Freshness score={o.freshnessScore} />
                </div>
                <div className="offer-side">
                  <dl className="offer-facts">
                    <dt className="sr-only">Price</dt>
                    <dd className="offer-price">{formatPrice(o.priceChhertum)}</dd>
                    <dt className="sr-only">In stock</dt>
                    <dd className="offer-qty">{formatQuantity(o.quantityAvailable)} in stock</dd>
                  </dl>
                  <Link href={reserveHref(product, o)} className={best ? "button button-small" : "button button-small button-outline"}>
                    Reserve<span className="sr-only"> at {o.pharmacyName}</span>
                  </Link>
                </div>
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
  /** When given, medicines nobody has get a "Notify me when it is back" button. */
  watch?: WatchOptions;
}

export function ResultsList({ query, results, substitutes, watch }: Props) {
  if (results.length === 0) {
    return (
      <div className="no-match" role="status">
        <p className="no-match-title">No medicines match “{query}”</p>
        <p className="muted">Check the spelling, or try the generic name, for example Paracetamol instead of a brand.</p>
      </div>
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
        <ProductCard key={r.productId} product={r} watch={watch} />
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
