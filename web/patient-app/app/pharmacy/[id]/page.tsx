import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { ArrowLeftIcon, InfoIcon, MapPinIcon } from "@/components/Icons";
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
    <div className="page">
      <Link href="/" className="back-link">
        <ArrowLeftIcon size={16} /> New search
      </Link>
      <article className="card detail-card">
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
        <a href={mapUrl} rel="noopener noreferrer" target="_blank" className="button button-outline">
          <MapPinIcon size={16} /> Open in map<span className="sr-only"> (opens in a new tab)</span>
        </a>
      </article>
      <p className="notice" role="note">
        <InfoIcon />
        <span>Reserving medicine and opening hours are coming soon.</span>
      </p>
    </div>
  );
}
