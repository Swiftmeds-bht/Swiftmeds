import { AlertIcon, CheckIcon, InfoIcon } from "../Icons";

import { REJECTED, SUSPENDED, type Pharmacy } from "@/lib/pharmacies";

const dateFormat = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "Asia/Thimphu" });
const when = (iso: string | undefined) => (iso && !Number.isNaN(Date.parse(iso)) ? dateFormat.format(new Date(iso)) : undefined);

/**
 * What a pharmacy that is not approved sees instead of the portal (SCRUM-142):
 * where its review stands and, if it was rejected or suspended, why.
 */
export function PharmacyStatusPanel({ pharmacy }: { pharmacy: Pharmacy }) {
  const registered = when(pharmacy.createdAt);
  if (pharmacy.status === REJECTED || pharmacy.status === SUSPENDED) {
    const rejected = pharmacy.status === REJECTED;
    return (
      <section className="status-panel status-panel-problem" aria-labelledby="pharmacy-status-heading">
        <span className="status-panel-icon" aria-hidden="true">
          <AlertIcon />
        </span>
        <h1 id="pharmacy-status-heading">{rejected ? "Your registration was not approved" : "Your pharmacy is suspended"}</h1>
        <p className="lead">
          {rejected
            ? `SwiftMed could not approve ${pharmacy.name || "your pharmacy"}, so patients cannot find it.`
            : `${pharmacy.name || "Your pharmacy"} is hidden from patients for now. Holds already made stay as they are.`}
        </p>
        {pharmacy.statusReason && (
          <div className="status-reason">
            <h2>The reason given</h2>
            <p>{pharmacy.statusReason}</p>
          </div>
        )}
        <p className="muted">If you think this is a mistake, contact SwiftMed and give your pharmacy&apos;s licence number.</p>
      </section>
    );
  }

  return (
    <section className="status-panel" aria-labelledby="pharmacy-status-heading">
      <span className="status-panel-icon" aria-hidden="true">
        <InfoIcon />
      </span>
      <h1 id="pharmacy-status-heading">Waiting for approval</h1>
      <p className="lead">
        Thank you for registering {pharmacy.name || "your pharmacy"}. A SwiftMed administrator checks every pharmacy before patients can find it.
      </p>
      <ol className="review-steps">
        <li className="review-step done">
          <CheckIcon />
          <span>
            <strong>Registered</strong>
            {registered ? ` on ${registered}` : ""}
          </span>
        </li>
        <li className="review-step current" aria-current="step">
          <span className="review-dot" aria-hidden="true" />
          <span>
            <strong>Licence and details checked</strong> by an administrator
          </span>
        </li>
        <li className="review-step">
          <span className="review-dot" aria-hidden="true" />
          <span>
            <strong>Patients find you</strong>: your stock, holds and price list open here
          </span>
        </li>
      </ol>
      <dl className="status-details">
        <div>
          <dt>Pharmacy</dt>
          <dd>{pharmacy.name || "—"}</dd>
        </div>
        <div>
          <dt>Dzongkhag</dt>
          <dd>{pharmacy.dzongkhag || "—"}</dd>
        </div>
        <div>
          <dt>Address</dt>
          <dd>{pharmacy.address || "—"}</dd>
        </div>
      </dl>
      <p className="muted">This page updates when you open it again.</p>
    </section>
  );
}
