import { SearchError, searchMedicines } from "@/lib/api";
import { formatDistance } from "@/lib/format";
import { MAX_QUERY_LENGTH, MIN_QUERY_LENGTH } from "@/lib/rules";
import type { GeoPoint } from "@/lib/types";

import { ResultsList } from "./ResultsList";
import { SearchProblem } from "./SearchProblem";

interface Props {
  query: string;
  point: GeoPoint;
  /** Where the search is from, for the heading: a town name or "your location". */
  label: string;
  radiusM: number;
  /** Link to run the same search again. */
  retryHref: string;
}

// Runs the search and shows the results, or says clearly why it could not.
// A server component: the page wraps it in <Suspense> so a loading state shows meanwhile.
export async function ResultsSection({ query, point, label, radiusM, retryHref }: Props) {
  if (query.length < MIN_QUERY_LENGTH || query.length > MAX_QUERY_LENGTH) {
    return <SearchProblem kind="bad_request" />;
  }

  let outcome;
  try {
    outcome = await searchMedicines({ query, location: point, radiusM });
  } catch (error) {
    if (!(error instanceof SearchError)) throw error; // unexpected: the error boundary handles it
    console.error(`search failed (${error.kind}):`, error.message);
    return <SearchProblem kind={error.kind} retryHref={retryHref} />;
  }

  const { source, data, degraded } = outcome;
  return (
    <>
      {source === "demo" && (
        <p className="notice" role="note">
          Showing demo data. Live search is used when GATEWAY_URL is set.
        </p>
      )}
      {degraded.length > 0 && (
        <p className="notice" role="note">
          Some details are unavailable right now, so prices, freshness or the order may be incomplete.
        </p>
      )}
      <h2 className="sr-only">
        Pharmacies within {formatDistance(radiusM)} of {label}
      </h2>
      <ResultsList query={query} results={data.results} substitutes={data.substitutes} />
    </>
  );
}
