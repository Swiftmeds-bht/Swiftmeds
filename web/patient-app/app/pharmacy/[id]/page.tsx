import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { ArrowLeftIcon, InfoIcon, MapPinIcon, SearchIcon, StoreIcon } from "@/components/Icons";
import { getPharmacy } from "@/lib/api";

type Params = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const pharmacy = getPharmacy((await params).id);
  return { title: pharmacy?.name ?? "Pharmacy" };
}

// Placeholder: opening hours and the stock list come in later sprints.
export default async function PharmacyPage({ params }: { params: Params }) {
  const pharmacy = getPharmacy((await params).id);
  if (!pharmacy) notFound();

  const mapUrl = `https://www.openstreetmap.org/?mlat=${pharmacy.location.latitude}&mlon=${pharmacy.location.longitude}#map=17/${pharmacy.location.latitude}/${pharmacy.location.longitude}`;

  return (
    <div className="page narrow pharmacy-page">
      <Link href="/" className="back-link">
        <ArrowLeftIcon size={16} /> New search
      </Link>
      <article className="panel detail-card">
        <div className="detail-head">
          <span className="detail-icon">
            <StoreIcon size={24} />
          </span>
          <div>
            <h1>{pharmacy.name}</h1>
            <p className="muted">Pharmacy in {pharmacy.dzongkhag}</p>
          </div>
        </div>
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
        <div className="detail-actions">
          <a href={mapUrl} rel="noopener noreferrer" target="_blank" className="button">
            <MapPinIcon size={16} /> Open in map<span className="sr-only"> (opens in a new tab)</span>
          </a>
          <Link href="/" className="button button-outline">
            <SearchIcon size={16} /> Find a medicine
          </Link>
        </div>
      </article>
      <p className="notice" role="note">
        <InfoIcon />
        <span>
          Opening hours and this pharmacy&apos;s full stock list are coming soon. To reserve, search for your medicine and press
          Reserve next to this pharmacy.
        </span>
      </p>
    </div>
  );
}
