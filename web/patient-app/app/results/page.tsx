import type { Metadata } from "next";

import { ResultsList } from "@/components/ResultsList";
import { SearchForm } from "@/components/SearchForm";
import { searchMedicines } from "@/lib/api";
import { formatDistance } from "@/lib/format";
import { parseRadius, placeById } from "@/lib/places";

export const metadata: Metadata = { title: "Search results" };

// Always render per request: stock changes all the time.
export const dynamic = "force-dynamic";

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export default async function ResultsPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const sp = await searchParams;
  const query = (first(sp.q) ?? "").trim();
  const place = placeById(first(sp.place));
  const radiusM = parseRadius(first(sp.radius));

  const { source, data } = query
    ? await searchMedicines({ query, location: place.location, radiusM })
    : { source: "demo" as const, data: { searchId: "", results: [], substitutes: [] } };

  return (
    <>
      <h1>Results</h1>
      <SearchForm query={query} placeId={place.id} radiusM={radiusM} />
      {source === "demo" && (
        <p className="notice" role="note">
          Showing demo data. Live search arrives when the gateway search route is ready.
        </p>
      )}
      <h2 className="sr-only">
        Pharmacies within {formatDistance(radiusM)} of {place.label}
      </h2>
      {query ? (
        <ResultsList query={query} results={data.results} substitutes={data.substitutes} />
      ) : (
        <p className="empty">Type a medicine name to search.</p>
      )}
    </>
  );
}
