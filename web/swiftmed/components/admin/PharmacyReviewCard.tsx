import { reviewPharmacyAction } from "@/app/admin/actions";
import { PENDING, VERIFIED, type Pharmacy } from "@/lib/pharmacies";

import { MapPinIcon } from "../Icons";

import { ReviewActions } from "./ReviewActions";

const dateFormat = new Intl.DateTimeFormat("en-GB", {
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "Asia/Thimphu",
});
const when = (iso: string | undefined) => (iso && !Number.isNaN(Date.parse(iso)) ? dateFormat.format(new Date(iso)) : "—");

/** One pharmacy as the administrator reviews it (SCRUM-142): what they need to check, and what they can do. */
export function PharmacyReviewCard({ pharmacy, tab }: { pharmacy: Pharmacy; tab: string }) {
  const loc = pharmacy.location;
  const mapUrl = loc
    ? `https://www.openstreetmap.org/?mlat=${loc.latitude}&mlon=${loc.longitude}#map=17/${loc.latitude}/${loc.longitude}`
    : undefined;
  const headingId = `pharmacy-${pharmacy.pharmacyId}`;
  return (
    <article className="review-card panel" aria-labelledby={headingId}>
      <header className="review-card-head">
        <h2 id={headingId}>{pharmacy.name || "Unnamed pharmacy"}</h2>
        <p className="muted">
          {pharmacy.dzongkhag || "No dzongkhag"} · registered {when(pharmacy.createdAt)}
        </p>
      </header>
      <dl className="review-facts">
        <div>
          <dt>Licence number</dt>
          <dd className="mono">{pharmacy.licenseNumber || "—"}</dd>
        </div>
        <div>
          <dt>Phone</dt>
          <dd>{pharmacy.phone ? <a href={`tel:${pharmacy.phone}`}>{pharmacy.phone}</a> : "—"}</dd>
        </div>
        <div>
          <dt>Address</dt>
          <dd>{pharmacy.address || "—"}</dd>
        </div>
        <div>
          <dt>Location</dt>
          <dd>
            {mapUrl ? (
              <a href={mapUrl} target="_blank" rel="noopener noreferrer" className="map-link">
                <MapPinIcon size={16} />
                {loc!.latitude.toFixed(4)}, {loc!.longitude.toFixed(4)}
                <span className="sr-only"> (opens a map in a new tab)</span>
              </a>
            ) : (
              "—"
            )}
          </dd>
        </div>
        {pharmacy.verifiedAt && (
          <div>
            <dt>Approved</dt>
            <dd>{when(pharmacy.verifiedAt)}</dd>
          </div>
        )}
        {pharmacy.statusReason && (
          <div className="review-reason-shown">
            <dt>Reason given</dt>
            <dd>{pharmacy.statusReason}</dd>
          </div>
        )}
      </dl>
      {(pharmacy.status === PENDING || pharmacy.status === VERIFIED) && (
        <ReviewActions
          action={reviewPharmacyAction}
          pharmacyId={pharmacy.pharmacyId}
          name={pharmacy.name}
          tab={tab}
          allowed={pharmacy.status === PENDING ? "review" : "suspend"}
        />
      )}
    </article>
  );
}
