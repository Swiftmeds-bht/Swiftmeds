import { lookupPharmacy } from "@/lib/api";
import { directionsUrl } from "@/lib/directions";

import { MapPinIcon } from "./Icons";

/** Where to collect, with directions (SCRUM-158). Says nothing if the pharmacy cannot be looked up. */
export async function GettingThere({ pharmacyId }: { pharmacyId?: string }) {
  if (!pharmacyId) return null;
  const found = await lookupPharmacy(pharmacyId);
  if (found.status !== "found") return null;
  const { pharmacy } = found;
  const directions = directionsUrl(pharmacy.location);
  if (!directions && !pharmacy.address) return null;
  return (
    <section className="panel getting-there" aria-labelledby="getting-there-heading">
      <h2 id="getting-there-heading">Getting there</h2>
      <p>
        <strong>{pharmacy.name}</strong>
        {pharmacy.address ? <span className="muted"> · {pharmacy.address}</span> : null}
      </p>
      {directions && (
        <a href={directions} rel="noopener noreferrer" target="_blank" className="button button-outline">
          <MapPinIcon size={16} /> Get directions<span className="sr-only"> (opens your maps app or a new tab)</span>
        </a>
      )}
    </section>
  );
}
