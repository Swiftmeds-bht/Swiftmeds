// Checks the pharmacy sign-up form (SCRUM-142) before anything is sent, so the
// pharmacist sees what to fix next to the field. The gateway checks again.
import type { PharmacyDetails } from "./pharmacies";
import type { GeoPoint } from "./types";

/**
 * Bhutan's 20 dzongkhags, each with the rough centre of its main town. The
 * centre is the pharmacy's location unless the pharmacist uses their device's
 * location instead. Spelled as in the seed data ("Chukha"), so Analytics groups
 * them the same way.
 */
export const DZONGKHAGS: { name: string; centre: GeoPoint }[] = [
  { name: "Bumthang", centre: { latitude: 27.5492, longitude: 90.7525 } },
  { name: "Chukha", centre: { latitude: 26.8516, longitude: 89.3884 } },
  { name: "Dagana", centre: { latitude: 27.0697, longitude: 89.8771 } },
  { name: "Gasa", centre: { latitude: 27.9061, longitude: 89.7269 } },
  { name: "Haa", centre: { latitude: 27.3685, longitude: 89.2918 } },
  { name: "Lhuentse", centre: { latitude: 27.6679, longitude: 91.1839 } },
  { name: "Mongar", centre: { latitude: 27.2747, longitude: 91.2396 } },
  { name: "Paro", centre: { latitude: 27.4305, longitude: 89.4133 } },
  { name: "Pemagatshel", centre: { latitude: 27.0379, longitude: 91.4031 } },
  { name: "Punakha", centre: { latitude: 27.5914, longitude: 89.8774 } },
  { name: "Samdrup Jongkhar", centre: { latitude: 26.8007, longitude: 91.5051 } },
  { name: "Samtse", centre: { latitude: 26.8993, longitude: 89.0995 } },
  { name: "Sarpang", centre: { latitude: 26.8639, longitude: 90.2674 } },
  { name: "Thimphu", centre: { latitude: 27.4728, longitude: 89.639 } },
  { name: "Trashigang", centre: { latitude: 27.3333, longitude: 91.5539 } },
  { name: "Trashiyangtse", centre: { latitude: 27.6116, longitude: 91.498 } },
  { name: "Trongsa", centre: { latitude: 27.5026, longitude: 90.5071 } },
  { name: "Tsirang", centre: { latitude: 27.0219, longitude: 90.1229 } },
  { name: "Wangdue Phodrang", centre: { latitude: 27.4863, longitude: 89.8993 } },
  { name: "Zhemgang", centre: { latitude: 27.2169, longitude: 90.6579 } },
];

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
