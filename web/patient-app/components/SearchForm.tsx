import { DEFAULT_RADIUS_M, PLACES, RADII_M } from "@/lib/places";
import { formatDistance } from "@/lib/format";

import { SearchIcon } from "./Icons";

interface Props {
  query?: string;
  placeId?: string;
  radiusM?: number;
  /** "hero" on the home page; "compact" sits above the results. */
  variant?: "hero" | "compact";
}

// A plain GET form: works without JavaScript and keeps the search in the URL,
// so results can be bookmarked and shared.
export function SearchForm({
  query = "",
  placeId = PLACES[0].id,
  radiusM = DEFAULT_RADIUS_M,
  variant = "hero",
}: Props) {
  return (
    <form
      action="/results"
      method="get"
      role="search"
      aria-label="Search for a medicine"
      className={variant === "compact" ? "search-form search-form-compact" : "search-form"}
    >
      <div className="search-bar">
        <SearchIcon size={20} />
        <label htmlFor="q" className="sr-only">
          Medicine name
        </label>
        <input
          id="q"
          name="q"
          type="search"
          required
          minLength={2}
          defaultValue={query}
          placeholder="Search a medicine, e.g. Paracetamol"
          autoComplete="off"
          aria-describedby="q-hint"
        />
        <button type="submit" className="button">
          Search
        </button>
      </div>
      <p id="q-hint" className="sr-only">
        Brand or generic name.
      </p>
      <div className="search-options">
        <div className="option-field">
          <label htmlFor="place">Near</label>
          <select id="place" name="place" defaultValue={placeId}>
            {PLACES.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </select>
        </div>
        <div className="option-field">
          <label htmlFor="radius">Within</label>
          <select id="radius" name="radius" defaultValue={String(radiusM)}>
            {RADII_M.map((r) => (
              <option key={r} value={r}>
                {formatDistance(r)}
              </option>
            ))}
          </select>
        </div>
      </div>
    </form>
  );
}
