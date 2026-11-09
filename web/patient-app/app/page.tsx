import { CheckIcon } from "@/components/Icons";
import { PickupTicket } from "@/components/PickupTicket";
import { SearchForm } from "@/components/SearchForm";

export default function HomePage() {
  return (
    <>
      <section className="home-band" aria-labelledby="home-heading">
        <div className="wrap home-band-inner">
          <div>
            <h1 id="home-heading">Find your medicine before you leave home</h1>
            <p className="lead">Live stock from verified pharmacies near you. Hold it, then collect it with a code.</p>
            <div className="search-panel">
              <SearchForm />
            </div>
          </div>

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
        <p className="how-note">
          <CheckIcon size={18} />
          <span>
            Nobody nearby has it? SwiftMed shows other brands with the same ingredient and strength. Ask the pharmacist before
            switching.
          </span>
        </p>
      </section>
    </>
  );
}
