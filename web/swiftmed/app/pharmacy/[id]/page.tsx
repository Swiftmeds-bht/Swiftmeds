import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { cache } from "react";

import { ArrowLeftIcon, InfoIcon, MapPinIcon, SearchIcon, StoreIcon } from "@/components/Icons";
import { lookupPharmacy } from "@/lib/api";
import { directionsUrl, openStreetMapUrl } from "@/lib/directions";

type Params = Promise<{ id: string }>;

export const dynamic = "force-dynamic";

// One gateway call per request, shared by the metadata and the page.
const lookup = cache((id: string) => lookupPharmacy(id));

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const result = await lookup((await params).id);
  return { title: result.status === "found" ? result.pharmacy.name : "Pharmacy" };
}

// Opening hours and the stock list come in later sprints.
export default async function PharmacyPage({ params }: { params: Params }) {
  const result = await lookup((await params).id);
  if (result.status === "not_found") notFound();

  if (result.status === "unavailable") {
    console.error(`pharmacy details unavailable (${result.kind})`);
    return (
      <div className="page narrow pharmacy-page">
        <Link href="/" className="back-link">
          <ArrowLeftIcon size={16} /> New search
        </Link>
        <div className="problem" role="alert">
          <h1>Pharmacy details are not available right now</h1>
          <p>
            We could not load this pharmacy&apos;s details. Its address and distance are shown with your search results, and you
            can still reserve from there.
          </p>
          <Link href="/" className="button">
            <SearchIcon size={16} /> Find a medicine
          </Link>
        </div>
      </div>
    );
  }

  const { pharmacy, source } = result;
  const directions = directionsUrl(pharmacy.location);
  const mapUrl = openStreetMapUrl(pharmacy.location);

  return (
    <div className="page narrow pharmacy-page">
      <Link href="/" className="back-link">
        <ArrowLeftIcon size={16} /> New search
      </Link>
      {source === "demo" && (
        <p className="notice" role="note">
          <InfoIcon />
          <span>Showing a demo pharmacy, not live data.</span>
        </p>
      )}
      <article className="panel detail-card">
        <div className="detail-head">
          <span className="detail-icon">
            <StoreIcon size={24} />
          </span>
          <div>
            <h1>{pharmacy.name}</h1>
            {pharmacy.dzongkhag && <p className="muted">Pharmacy in {pharmacy.dzongkhag}</p>}
          </div>
        </div>
        <dl className="details">
          {pharmacy.address && (
            <div>
              <dt>Address</dt>
              <dd>{pharmacy.address}</dd>
            </div>
          )}
          {pharmacy.dzongkhag && (
            <div>
              <dt>Dzongkhag</dt>
              <dd>{pharmacy.dzongkhag}</dd>
            </div>
          )}
          {pharmacy.phone && (
            <div>
              <dt>Phone</dt>
              <dd>
                <a href={`tel:${pharmacy.phone.replace(/[^\d+]/g, "")}`}>{pharmacy.phone}</a>
              </dd>
            </div>
          )}
        </dl>
        <div className="detail-actions">
          {directions && (
            <a href={directions} rel="noopener noreferrer" target="_blank" className="button">
              <MapPinIcon size={16} /> Get directions<span className="sr-only"> (opens your maps app or a new tab)</span>
            </a>
          )}
          <Link href="/" className={directions ? "button button-outline" : "button"}>
            <SearchIcon size={16} /> Find a medicine
          </Link>
        </div>
        {mapUrl && (
          <p className="hint">
            <a href={mapUrl} rel="noopener noreferrer" target="_blank">
              See it on OpenStreetMap<span className="sr-only"> (opens in a new tab)</span>
            </a>
          </p>
        )}
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
