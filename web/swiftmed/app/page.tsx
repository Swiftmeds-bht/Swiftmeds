import Link from "next/link";

import { CheckIcon, SearchIcon } from "@/components/Icons";
import { PickupTicket } from "@/components/PickupTicket";
import { SearchForm } from "@/components/SearchForm";
import { DEFAULT_RADIUS_M, PLACES } from "@/lib/places";

// Medicines people often look for (all in the demo catalogue): one tap searches
// them around the default town.
const COMMON_SEARCHES = ["Paracetamol", "Amoxicillin", "Ibuprofen", "Cetirizine", "Omeprazole", "Metformin", "Amlodipine"];

const searchHref = (q: string) =>
  `/results?${new URLSearchParams({ q, place: PLACES[0].id, radius: String(DEFAULT_RADIUS_M) })}`;

/**
 * Home (laid out in SCRUM-139 like common booking sites): a centred promise,
 * one wide search card, and quick searches under it; then how it works.
 */
export default function HomePage() {
  return (
    <>
      <section className="home-hero" aria-labelledby="home-heading">
        <div className="wrap home-hero-inner">
          <h1 id="home-heading">Find your medicine before you leave home</h1>
          <p className="lead">Live stock from verified pharmacies near you. Reserve it, then collect it with a code.</p>
          <div className="search-card">
            <SearchForm />
          </div>
          <ul className="quick-pills" aria-label="Common searches">
            {COMMON_SEARCHES.map((name) => (
              <li key={name}>
                <Link href={searchHref(name)} className="pill">
                  <SearchIcon size={15} />
                  {name}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className="how wrap" aria-labelledby="how-heading">
        <h2 id="how-heading">How SwiftMed works</h2>
        <ol className="steps">
          <li className="step">
            <span className="step-num" aria-hidden="true">
              1
            </span>
            <h3>Search</h3>
            <p>See which verified pharmacies near you have it, and how recently their stock was confirmed.</p>
          </li>
          <li className="step">
            <span className="step-num" aria-hidden="true">
              2
            </span>
            <h3>Reserve</h3>
            <p>Hold 1 to 5 units. The pharmacy checks its stock, then you get a pickup code.</p>
          </li>
          <li className="step">
            <span className="step-num" aria-hidden="true">
              3
            </span>
            <h3>Collect</h3>
            <p>Show the code at the counter before the time runs out. You pay at the pharmacy.</p>
          </li>
        </ol>

        <div className="how-bottom">
          <p className="how-note">
            <CheckIcon size={18} />
            <span>
              Nobody nearby has it? SwiftMed shows other brands with the same ingredient and strength. Ask the pharmacist before
              switching.
            </span>
          </p>

          {/* What a hold looks like, so the pickup code is not a surprise later. */}
          <figure className="home-example">
            <div className="example-hold" aria-hidden="true">
              <div className="hold-head">
                <p className="hold-title example-title">Demo Paracetamol 500 500mg</p>
                <div className="hold-meta">
                  <span className="badge badge-high">Ready to collect</span>
                  <p className="muted">2 units · Demo Pharmacy Thimphu 03</p>
                </div>
              </div>
              <PickupTicket code="482913" small>
                <p className="countdown">
                  <span className="countdown-time">29:41</span>
                  <span className="countdown-label">left until 14:30</span>
                </p>
              </PickupTicket>
            </div>
            <figcaption>Example: the code you show at the counter</figcaption>
          </figure>
        </div>
      </section>

      <section className="for-pharmacies wrap" aria-labelledby="pharmacies-heading">
        <div>
          <h2 id="pharmacies-heading">Run a pharmacy?</h2>
          <p className="muted">Show patients what you have in stock, and hand over reserved medicine with a code. Every pharmacy is checked before patients can find it.</p>
        </div>
        <div className="for-pharmacies-actions">
          <Link href="/register/pharmacy" className="button">
            Register your pharmacy
          </Link>
          <Link href="/login?next=%2Fpharmacy-portal" className="button button-outline">
            Log in
          </Link>
        </div>
      </section>
    </>
  );
}
