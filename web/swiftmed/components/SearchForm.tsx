"use client";

import { useRouter } from "next/navigation";
import { useTransition, type FormEvent } from "react";

import { formatDistance } from "@/lib/format";
import { DEFAULT_RADIUS_M, PLACES, RADII_M } from "@/lib/places";
import type { GeoPoint } from "@/lib/types";

import { MapPinIcon, SearchIcon } from "./Icons";
import { LocationPicker } from "./LocationPicker";
import { MedicineCombobox } from "./MedicineCombobox";

interface Props {
  query?: string;
  placeId?: string;
  radiusM?: number;
  /** Set when the current results were searched from the device location. */
  deviceLocation?: GeoPoint;
  /** "hero" on the home page; "compact" sits above the results. */
  variant?: "hero" | "compact";
}

// A real GET form, so it works without JavaScript and every search is a shareable
// URL. With JavaScript it navigates in place and shows a "Searching…" state.
export function SearchForm({
  query = "",
  placeId = PLACES[0].id,
  radiusM = DEFAULT_RADIUS_M,
  deviceLocation,
  variant = "hero",
}: Props) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const compact = variant === "compact";
  const placeLabel = PLACES.find((p) => p.id === placeId)?.label ?? PLACES[0].label;

  const areaFields = (
    <>
      <LocationPicker placeId={placeId} deviceLocation={deviceLocation} />
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
    </>
  );

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const params = new URLSearchParams();
    for (const [name, value] of new FormData(event.currentTarget)) {
      if (typeof value === "string" && value !== "") params.set(name, value);
    }
    startTransition(() => router.push(`/results?${params}`));
  }

  return (
    <form
      action="/results"
      method="get"
      role="search"
      aria-label="Search for a medicine"
      className={compact ? "search-form search-form-compact" : "search-form"}
      onSubmit={onSubmit}
    >
      <MedicineCombobox defaultValue={query} />
      {compact ? <details className="search-area">
          {/* Closed, the fields still submit: the area is one tap away, the results stay near the top. */}
          <summary>
            <MapPinIcon size={16} />
            <span>
              Near {deviceLocation ? "your location" : placeLabel}, within {formatDistance(radiusM)}
            </span>
            <span className="search-area-change">Change</span>
          </summary>
          <div className="search-area-fields">{areaFields}</div>
        </details> : areaFields}
      <button type="submit" className="button button-lg search-submit" disabled={pending}>
        <SearchIcon size={18} />
        {/* On the home page's search card the button is a round icon; the text stays its name. */}
        <span className="search-submit-text">{pending ? "Searching…" : "Search"}</span>
      </button>
    </form>
  );
}
