"use client";

import "leaflet/dist/leaflet.css";

import { useEffect, useId, useRef, useState } from "react";

import type { MapPin } from "@/lib/map-points";
import type { GeoPoint } from "@/lib/types";

interface Props {
  /** Where the search is from. */
  center: GeoPoint;
  /** The search place, for the "you searched here" marker. */
  label: string;
  pins: MapPin[];
}

const escape = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);

/**
 * The pharmacies on a map (SCRUM-158): OpenStreetMap tiles through Leaflet,
 * loaded only in the browser after the results list. It adds to the list,
 * which has the same information for keyboard and screen-reader users; if the
 * map cannot load, the list is all there is.
 */
export function ResultsMap({ center, label, pins }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  const descId = useId();

  useEffect(() => {
    let map: import("leaflet").Map | undefined;
    let cancelled = false;
    import("leaflet")
      .then((L) => {
        if (cancelled || !ref.current) return;
        map = L.map(ref.current, { scrollWheelZoom: false, attributionControl: true });
        L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
          maxZoom: 19,
          attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
        }).addTo(map);

        const pinIcon = (n: number) =>
          // Leaflet positions the marker element with a transform, so the
          // rotated teardrop is an inner element.
          L.divIcon({
            className: "map-pin",
            html: `<span class="map-pin-shape"><span>${n}</span></span>`,
            iconSize: [30, 30],
            iconAnchor: [15, 30],
            popupAnchor: [0, -28],
          });
        const here = L.divIcon({ className: "map-here", iconSize: [18, 18], iconAnchor: [9, 9] });

        L.marker([center.latitude, center.longitude], { icon: here, keyboard: false, title: `You searched near ${label}` })
          .addTo(map)
          .bindTooltip(`Near ${escape(label)}`);

        const bounds = L.latLngBounds([[center.latitude, center.longitude]]);
        pins.forEach((p, i) => {
          const items = p.medicines
            .map((m) => `<li>${escape(m.label)} <a href="${escape(m.reserveHref)}">Reserve</a></li>`)
            .join("");
          L.marker([p.location.latitude, p.location.longitude], { icon: pinIcon(i + 1), title: p.name, alt: p.name })
            .addTo(map!)
            .bindPopup(
              `<strong><a href="/pharmacy/${encodeURIComponent(p.pharmacyId)}">${escape(p.name)}</a></strong>` +
                (p.address ? `<br><span class="map-popup-muted">${escape(p.address)}</span>` : "") +
                `<ul class="map-popup-list">${items}</ul>`,
            );
          bounds.extend([p.location.latitude, p.location.longitude]);
        });
        map.fitBounds(bounds, { padding: [36, 36], maxZoom: 16 });
      })
      .catch(() => setFailed(true));
    return () => {
      cancelled = true;
      map?.remove();
    };
  }, [center.latitude, center.longitude, label, pins]);

  if (failed) return null;
  return (
    <section className="results-map" aria-labelledby={`${descId}-title`}>
      <h3 id={`${descId}-title`} className="sr-only">
        Map of the pharmacies in the list below
      </h3>
      <div ref={ref} className="results-map-canvas" role="region" aria-label="Map of the pharmacies in the list below" />
      <p className="hint results-map-legend">
        <span className="map-legend-pin" aria-hidden="true">
          1
        </span>{" "}
        Pharmacies with your medicine, nearest first. Tap a pin to reserve.
      </p>
    </section>
  );
}
