import { SearchForm } from "@/components/SearchForm";

export default function HomePage() {
  return (
    <>
      <h1>Find your medicine before you leave home</h1>
      <p className="lead">See which nearby pharmacies have it in stock right now.</p>
      <SearchForm />
    </>
  );
}
