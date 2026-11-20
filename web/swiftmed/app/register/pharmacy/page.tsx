import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { registerPharmacyAction } from "@/app/actions";
import { AudienceSwitch } from "@/components/AudienceSwitch";
import { InfoIcon } from "@/components/Icons";
import { PharmacySignupForm } from "@/components/PharmacySignupForm";
import { getSession } from "@/lib/auth";
import { gatewayUrl } from "@/lib/data-source";
import { PATIENT_SIGNUP, PHARMACY_SIGNUP, PORTAL_PATH, homeAfterSignIn, isAdmin, isPharmacyStaff } from "@/lib/roles";

export const metadata: Metadata = { title: "Register your pharmacy" };

const STEPS = [
  { title: "Register", text: "Your login and your pharmacy's details, on this page." },
  { title: "We check", text: "A SwiftMed administrator checks your licence and details." },
  { title: "Patients find you", text: "Once approved, your stock shows in search and patients can reserve." },
];

/**
 * Pharmacy sign-up (SCRUM-142). Signed out: a login and the pharmacy's
 * details. Signed in as a patient: only the pharmacy's details (the account
 * becomes the pharmacy's account). Pharmacy staff and admins go to their pages.
 */
export default async function PharmacySignupPage() {
  const session = await getSession();
  if (session && (isAdmin(session) || isPharmacyStaff(session))) redirect(homeAfterSignIn(session, PORTAL_PATH));
  const demo = gatewayUrl() === "";

  return (
    <div className="page narrow auth-page pharmacy-signup">
      {!session && (
        <AudienceSwitch
          label="Sign up as"
          options={[
            { href: PATIENT_SIGNUP, label: "Patient", current: false },
            { href: PHARMACY_SIGNUP, label: "Pharmacy", current: true },
          ]}
        />
      )}
      <h1>Register your pharmacy</h1>
      <p className="lead">Show patients what you have in stock and let them reserve it. Every pharmacy is checked before patients can find it.</p>
      <ol className="signup-steps">
        {STEPS.map((s, i) => (
          <li key={s.title}>
            <span className="signup-step-number" aria-hidden="true">
              {i + 1}
            </span>
            <span>
              <strong>{s.title}</strong> {s.text}
            </span>
          </li>
        ))}
      </ol>
      {demo ? (
        <p className="notice" role="note">
          <InfoIcon />
          <span>Pharmacy sign-up needs the live service. This copy of SwiftMed runs on demo data.</span>
        </p>
      ) : (
        <>
          {session && (
            <p className="notice" role="note">
              <InfoIcon />
              <span>
                The account {session.email} becomes your pharmacy&apos;s account. After you register, it opens the pharmacy portal instead of
                patient pages.
              </span>
            </p>
          )}
          <div className="panel auth-card">
            <PharmacySignupForm action={registerPharmacyAction} withAccount={!session} />
          </div>
        </>
      )}
    </div>
  );
}
