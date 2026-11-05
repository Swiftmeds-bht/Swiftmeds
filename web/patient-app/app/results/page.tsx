import type { Metadata } from "next";
import { Suspense } from "react";

import { ResultsSection } from "@/components/ResultsSection";
import { ResultsSkeleton } from "@/components/ResultsSkeleton";
import { SearchForm } from "@/components/SearchForm";
import { parseRadius, placeById, resolveLocation } from "@/lib/places";

// Always render per request: stock changes all the time.
export const dynamic = "force-dynamic";

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function generateMetadata({ searchParams }: { searchParams: SearchParamsIn }): Promise<Metadata> {
  const query = (first((await searchParams).q) ?? "").trim();
  return { title: query ? `Results for ${query}` : "Search results" };
}

export default async function ResultsPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const sp = await searchParams;
  const query = (first(sp.q) ?? "").trim();
  const place = placeById(first(sp.place));
  const location = resolveLocation(first(sp.lat), first(sp.lng), first(sp.place));
  const radiusM = parseRadius(first(sp.radius));

  // A link that repeats this exact search, for "Try again".
  const retry = new URLSearchParams();
  for (const name of ["q", "lat", "lng", "place", "radius"]) {
    const value = first(sp[name]);
    if (value) retry.set(name, value);
  }

  return (
    <div className="page">
      <h1>Results</h1>
      <SearchForm
        query={query}
        placeId={place.id}
        radiusM={radiusM}
        deviceLocation={location.fromDevice ? location.point : undefined}
        variant="compact"
      />
      {query ? (
        // The key makes a new search show the loading state again instead of the old results.
        <Suspense key={retry.toString()} fallback={<ResultsSkeleton />}>
          <ResultsSection
            query={query}
            point={location.point}
            label={location.label}
            radiusM={radiusM}
            retryHref={`/results?${retry}`}
          />
        </Suspense>
      ) : (
        <p className="empty">Type a medicine name to search.</p>
      )}
    </div>
  );
}
