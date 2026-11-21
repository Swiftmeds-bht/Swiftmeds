"use client";

import { useId, useState, useSyncExternalStore } from "react";

import { PLACES, roundCoordinate } from "@/lib/places";
import type { GeoPoint } from "@/lib/types";

import { LocateIcon, MapPinIcon } from "./Icons";

type Status = "idle" | "locating" | "found" | "denied" | "timeout" | "unavailable";

const MESSAGES: Record<Status, string> = {
  idle: "",
  locating: "Finding your location…",
  found: "Using your location.",
  denied: "Location is blocked for this site. Choose a town instead, or allow location in your browser settings.",
  timeout: "Finding your location took too long. Choose a town instead.",
  unavailable: "Your location is not available right now. Choose a town instead.",
};

// GeolocationPositionError codes.
const PERMISSION_DENIED = 1;
const TIMEOUT = 3;

const subscribeNothing = () => () => {};
const supportsLocation = () => typeof navigator !== "undefined" && "geolocation" in navigator;

interface Props {
  placeId: string;
  /** Set when the page was opened from a search that used the device location. */
  deviceLocation?: GeoPoint;
}

// "Near": the patient shares their device location, or picks a town (the manual fallback).
// The town list works without JavaScript; the location button only appears once JavaScript runs.
export function LocationPicker({ placeId, deviceLocation }: Props) {
  const id = useId();
  const selectId = `${id}-place`;
  const statusId = `${id}-status`;

  const canLocate = useSyncExternalStore(subscribeNothing, supportsLocation, () => false);
  const [device, setDevice] = useState<GeoPoint | undefined>(deviceLocation);
  const [status, setStatus] = useState<Status>(deviceLocation ? "found" : "idle");

  function locate() {
    setStatus("locating");
    navigator.geolocation.getCurrentPosition(
      (position) => {
        setDevice({
          latitude: roundCoordinate(position.coords.latitude),
          longitude: roundCoordinate(position.coords.longitude),
        });
        setStatus("found");
      },
      (error) => {
        setStatus(error.code === PERMISSION_DENIED ? "denied" : error.code === TIMEOUT ? "timeout" : "unavailable");
      },
      { timeout: 10_000, maximumAge: 5 * 60_000 },
    );
  }

  function useTown() {
    setDevice(undefined);
    setStatus("idle");
  }

  return (
    <div className="field">
      <label htmlFor={selectId}>Near</label>
      <select id={selectId} name="place" defaultValue={placeId} disabled={Boolean(device)} aria-describedby={statusId}>
        {PLACES.map((p) => (
          <option key={p.id} value={p.id}>
            {p.label}
          </option>
        ))}
      </select>
      {device && (
        <>
          <input type="hidden" name="lat" value={device.latitude} />
          <input type="hidden" name="lng" value={device.longitude} />
        </>
      )}
      {canLocate &&
        (device ? (
          <button type="button" className="link-button" onClick={useTown}>
            <MapPinIcon size={16} />
            Use a town instead
          </button>
        ) : (
          <button type="button" className="link-button" onClick={locate} disabled={status === "locating"}>
            <LocateIcon size={16} />
            Use my location
          </button>
        ))}
      <p id={statusId} role="status" className="hint">
        {MESSAGES[status]}
      </p>
    </div>
  );
}
