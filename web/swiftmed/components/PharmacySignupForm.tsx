"use client";

import Link from "next/link";
import { useActionState, useId, useState } from "react";

import type { PharmacySignupState } from "@/app/actions";
import { DZONGKHAGS, EMPTY_PHARMACY_FORM } from "@/lib/pharmacy-form";
import { STAFF_LOGIN } from "@/lib/roles";

import { LocateIcon } from "./Icons";

interface Props {
  action: (state: PharmacySignupState, form: FormData) => Promise<PharmacySignupState>;
  /** Ask for a new login too (signed out). Off when a patient account registers its pharmacy. */
  withAccount: boolean;
}

type Spot = { latitude: number; longitude: number } | undefined;

/**
 * Pharmacy sign-up (SCRUM-142): the login, then the pharmacy. A plain form
 * posting to a server action, so it works before JavaScript loads; only "Use my
 * current location" needs JavaScript, and without it the pharmacy is placed at
 * its dzongkhag's centre.
 */
export function PharmacySignupForm({ action, withAccount }: Props) {
  const [state, formAction, pending] = useActionState(action, {});
  const id = useId();
  const values = state.values ?? EMPTY_PHARMACY_FORM;
  const [spot, setSpot] = useState<Spot>(() =>
    values.latitude && values.longitude ? { latitude: Number(values.latitude), longitude: Number(values.longitude) } : undefined,
  );
  const [locating, setLocating] = useState<"idle" | "busy" | "failed">("idle");

  function useMyLocation() {
    if (!("geolocation" in navigator)) {
      setLocating("failed");
      return;
    }
    setLocating("busy");
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setSpot({ latitude: Number(pos.coords.latitude.toFixed(6)), longitude: Number(pos.coords.longitude.toFixed(6)) });
        setLocating("idle");
      },
      () => setLocating("failed"),
      { enableHighAccuracy: true, timeout: 10_000 },
    );
  }

  const field = (name: string) => `${id}-${name}`;
  const errorId = `${id}-error`;

  return (
    <form action={formAction} className="auth-form pharmacy-form" aria-describedby={state.error ? errorId : undefined}>
      {withAccount && (
        <fieldset className="form-section">
          <legend>Your login</legend>
          <p className="hint">You use it to log in to the pharmacy portal.</p>
          <div className="form-field">
            <label htmlFor={field("email")}>Email</label>
            <input id={field("email")} name="email" type="email" autoComplete="email" required defaultValue={state.email} />
          </div>
          <div className="form-row">
            <div className="form-field">
              <label htmlFor={field("password")}>Password</label>
              <input
                id={field("password")}
                name="password"
                type="password"
                autoComplete="new-password"
                minLength={8}
                maxLength={72}
                required
                aria-describedby={field("password-hint")}
              />
              <p id={field("password-hint")} className="hint">
                At least 8 characters.
              </p>
            </div>
            <div className="form-field">
              <label htmlFor={field("confirm")}>Repeat password</label>
              <input id={field("confirm")} name="confirm" type="password" autoComplete="new-password" minLength={8} maxLength={72} required />
            </div>
          </div>
        </fieldset>
      )}

      <fieldset className="form-section">
        <legend>Your pharmacy</legend>
        <div className="form-field">
          <label htmlFor={field("name")}>Pharmacy name</label>
          <input id={field("name")} name="name" autoComplete="organization" maxLength={200} required defaultValue={values.name} />
        </div>
        <div className="form-row">
          <div className="form-field">
            <label htmlFor={field("licenseNumber")}>Licence number</label>
            <input
              id={field("licenseNumber")}
              name="licenseNumber"
              maxLength={200}
              required
              defaultValue={values.licenseNumber}
              aria-describedby={field("licence-hint")}
            />
            <p id={field("licence-hint")} className="hint">
              As on your licence from the Bhutan Medicines and Health Products Authority.
            </p>
          </div>
          <div className="form-field">
            <label htmlFor={field("phone")}>Phone</label>
            <input id={field("phone")} name="phone" type="tel" autoComplete="tel" required defaultValue={values.phone} />
          </div>
        </div>
        <div className="form-field">
          <label htmlFor={field("address")}>Street address</label>
          <input id={field("address")} name="address" autoComplete="street-address" maxLength={200} required defaultValue={values.address} />
        </div>
        <div className="form-field">
          <label htmlFor={field("dzongkhag")}>Dzongkhag</label>
          <select id={field("dzongkhag")} name="dzongkhag" required defaultValue={values.dzongkhag}>
            <option value="" disabled>
              Choose…
            </option>
            {DZONGKHAGS.map((d) => (
              <option key={d.name} value={d.name}>
                {d.name}
              </option>
            ))}
          </select>
        </div>
      </fieldset>

      <fieldset className="form-section">
        <legend>Where patients find you</legend>
        <input type="hidden" name="latitude" value={spot ? String(spot.latitude) : ""} />
        <input type="hidden" name="longitude" value={spot ? String(spot.longitude) : ""} />
        {spot ? (
          <p className="location-box" role="status">
            <span>
              Your device&apos;s location: {spot.latitude.toFixed(4)}, {spot.longitude.toFixed(4)}
            </span>
            <button type="button" className="link-button" onClick={() => setSpot(undefined)}>
              Use the dzongkhag&apos;s centre instead
            </button>
          </p>
        ) : (
          <div className="location-box">
            <p>
              Without it, your pharmacy is placed at the centre of its dzongkhag. For the exact spot, press the button while you are at
              the pharmacy.
            </p>
            <button type="button" className="button button-outline button-small" onClick={useMyLocation} disabled={locating === "busy"}>
              <LocateIcon />
              {locating === "busy" ? "Finding you…" : "Use my current location"}
            </button>
            {locating === "failed" && (
              <p className="hint" role="status">
                Your device did not share its location. You can still register; the dzongkhag&apos;s centre is used.
              </p>
            )}
          </div>
        )}
      </fieldset>

      {state.error && (
        <p id={errorId} className="form-error" role="alert">
          {state.error}
        </p>
      )}
      <button type="submit" className="button button-block" disabled={pending}>
        {pending ? "Registering…" : "Register pharmacy"}
      </button>
      {withAccount && (
        <p className="auth-switch">
          Already registered? <Link href={STAFF_LOGIN}>Log in to the pharmacy portal</Link>
        </p>
      )}
    </form>
  );
}
