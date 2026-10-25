import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getPharmacy } from "@/lib/api";

type Params = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const pharmacy = getPharmacy((await params).id);
  return { title: pharmacy?.name ?? "Pharmacy" };
}

// Placeholder: opening hours, stock list and reservations come in later sprints.
export default async function PharmacyPage({ params }: { params: Params }) {
  const pharmacy = getPharmacy((await params).id);
  if (!pharmacy) notFound();

  const mapUrl = `https://www.openstreetmap.org/?mlat=${pharmacy.location.latitude}&mlon=${pharmacy.location.longitude}#map=17/${pharmacy.location.latitude}/${pharmacy.location.longitude}`;

  return (
    <>
      <p>
        <Link href="/">← New search</Link>
      </p>
      <h1>{pharmacy.name}</h1>
      <dl className="details">
        <div>
          <dt>Address</dt>
          <dd>{pharmacy.address}</dd>
        </div>
        <div>
          <dt>Dzongkhag</dt>
          <dd>{pharmacy.dzongkhag}</dd>
        </div>
      </dl>
      <p>
        <a href={mapUrl} rel="noopener noreferrer" target="_blank">
          Open in map<span className="sr-only"> (opens in a new tab)</span>
        </a>
      </p>
      <p className="notice" role="note">
        Reserving medicine and opening hours are coming soon.
      </p>
    </>
  );
}
