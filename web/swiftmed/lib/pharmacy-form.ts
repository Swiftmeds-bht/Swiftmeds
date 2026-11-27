// Checks the pharmacy sign-up form (SCRUM-142) before anything is sent, so the
// pharmacist sees what to fix next to the field. The gateway checks again.
import type { PharmacyDetails } from "./pharmacies";
import { PLACES } from "./places";
import type { GeoPoint } from "./types";

/**
 * Bhutan's 20 dzongkhags, each with the rough centre of its main town: the
 * same list patients search from (lib/places.ts, SCRUM-153), sorted by name.
 * The centre is the pharmacy's location unless the pharmacist uses their
 * device's location instead.
 */
export const DZONGKHAGS: { name: string; centre: GeoPoint }[] = PLACES.map((p) => ({ name: p.dzongkhag, centre: p.location })).sort(
  (a, b) => a.name.localeCompare(b.name),
);

export interface PharmacyFormValues {
  name: string;
  licenseNumber: string;
  phone: string;
  address: string;
  dzongkhag: string;
  latitude: string;
  longitude: string;
}

export const EMPTY_PHARMACY_FORM: PharmacyFormValues = {
  name: "",
  licenseNumber: "",
  phone: "",
  address: "",
  dzongkhag: "",
  latitude: "",
  longitude: "",
};

// Bhutanese landlines start with 0 (02 334455); mobiles with 17 or 77.
const PHONE = /^\+?[0-9]{7,15}$/;
const MAX = 200;

/** The typed values, trimmed, from a submitted form. */
export function readPharmacyForm(form: FormData): PharmacyFormValues {
  const text = (name: keyof PharmacyFormValues) => {
    const value = form.get(name);
    return typeof value === "string" ? value.trim() : "";
  };
  return {
    name: text("name"),
    licenseNumber: text("licenseNumber"),
    phone: text("phone"),
    address: text("address"),
    dzongkhag: text("dzongkhag"),
    latitude: text("latitude"),
    longitude: text("longitude"),
  };
}

/** The details to send, or the first problem in the form's order. */
export function checkPharmacyForm(values: PharmacyFormValues): { details: PharmacyDetails } | { error: string } {
  if (!values.name) return { error: "Enter the pharmacy's name." };
  if (!values.licenseNumber) return { error: "Enter the pharmacy's licence number." };
  const phone = values.phone.replace(/[\s\-.()]/g, "");
  if (!PHONE.test(phone)) return { error: "Enter a phone number with 7 to 15 digits, for example 02334455 or 17123456." };
  if (!values.address) return { error: "Enter the pharmacy's street address." };
  for (const [label, value] of [["name", values.name], ["licence number", values.licenseNumber], ["address", values.address]]) {
    if (value.length > MAX) return { error: `The ${label} is too long (${MAX} characters at most).` };
  }
  const dzongkhag = DZONGKHAGS.find((d) => d.name === values.dzongkhag);
  if (!dzongkhag) return { error: "Choose the pharmacy's dzongkhag." };

  let location = dzongkhag.centre;
  if (values.latitude || values.longitude) {
    const latitude = Number(values.latitude);
    const longitude = Number(values.longitude);
    if (!Number.isFinite(latitude) || !Number.isFinite(longitude) || Math.abs(latitude) > 90 || Math.abs(longitude) > 180) {
      return { error: "The location from your device is not valid. Use it again, or clear it to use the dzongkhag's centre." };
    }
    location = { latitude, longitude };
  }
  return {
    details: { name: values.name, licenseNumber: values.licenseNumber, phone, address: values.address, dzongkhag: dzongkhag.name, location },
  };
}
