import { CheckIcon } from "@/components/Icons";
import { SearchForm } from "@/components/SearchForm";

export default function HomePage() {
  return (
    <section className="hero">
      <h1>
        Find your medicine
        <br />
        before you leave home
      </h1>
      <p className="lead">Live stock from verified pharmacies near you.</p>
      <SearchForm />
      <ul className="trust-points">
        <li>
          <CheckIcon size={16} /> Verified pharmacies only
        </li>
        <li>
          <CheckIcon size={16} /> Stock updated through the day
        </li>
        <li>
          <CheckIcon size={16} /> Other brands with the same ingredient
        </li>
      </ul>
    </section>
  );
}
