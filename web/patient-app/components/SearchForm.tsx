import { DEFAULT_RADIUS_M, PLACES, RADII_M } from "@/lib/places";
import { formatDistance } from "@/lib/format";

interface Props {
  query?: string;
  placeId?: string;
  radiusM?: number;
}

// A plain GET form: works without JavaScript and keeps the search in the URL,
// so results can be bookmarked and shared.
export function SearchForm({ query = "", placeId = PLACES[0].id, radiusM = DEFAULT_RADIUS_M }: Props) {
  return (
    <form action="/results" method="get" role="search" aria-label="Search for a medicine" className="search-form">
      <div className="field field-wide">
        <label htmlFor="q">Medicine name</label>
        <input
          id="q"
          name="q"
          type="search"
          required
          minLength={2}
          defaultValue={query}
          placeholder="e.g. Paracetamol"
          autoComplete="off"
          aria-describedby="q-hint"
        />
        <p id="q-hint" className="hint">
          Brand or generic name.
        </p>
      </div>
      <div className="field">
        <label htmlFor="place">Near</label>
        <select id="place" name="place" defaultValue={placeId}>
          {PLACES.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label htmlFor="radius">Within</label>
        <select id="radius" name="radius" defaultValue={String(radiusM)}>
          {RADII_M.map((r) => (
            <option key={r} value={r}>
              {formatDistance(r)}
            </option>
          ))}
        </select>
      </div>
      <button type="submit" className="button">
        Search
      </button>
    </form>
  );
}
