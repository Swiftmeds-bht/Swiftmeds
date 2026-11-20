import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { loginAction } from "@/app/actions";
import { AudienceSwitch } from "@/components/AudienceSwitch";
import { AuthForm } from "@/components/AuthForm";
import { InfoIcon } from "@/components/Icons";
import { getSession } from "@/lib/auth";
import { holdsBackend } from "@/lib/holds";
import { PATIENT_LOGIN, PHARMACY_SIGNUP, STAFF_LOGIN, homeAfterSignIn, isPortalPath } from "@/lib/roles";
import { safeNext } from "@/lib/session";

export const metadata: Metadata = { title: "Log in" };

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

/**
 * One login page for everyone (SCRUM-139). The switch at the top (SCRUM-140)
 * only changes the words and where the page goes afterwards: pharmacy staff go
 * on to the portal, patients to `next`. The account decides in the end.
 */
export default async function LoginPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const sp = await searchParams;
  const next = safeNext(first(sp.next));
  const session = await getSession();
  if (session) redirect(homeAfterSignIn(session, next));
  const demo = holdsBackend().source === "demo";
  const staff = isPortalPath(next);

  return (
    <div className="page narrow auth-page">
      <AudienceSwitch
        label="Log in as"
        options={[
          { href: PATIENT_LOGIN, label: "Patient", current: !staff },
          { href: STAFF_LOGIN, label: "Pharmacy staff", current: staff },
        ]}
      />
      <h1>{staff ? "Pharmacy log in" : "Log in"}</h1>
      <p className="lead">
        {staff
          ? "See the holds waiting at your pharmacy, collect them and keep your stock up to date."
          : "Reserve medicine and follow your holds."}
      </p>
      {first(sp.registered) && (
        <p className="notice" role="note">
          <InfoIcon />
          <span>Your pharmacy is registered. Log in to see where its review stands.</span>
        </p>
      )}
      {first(sp.expired) && (
        <p className="notice notice-warning" role="note">
          <InfoIcon />
          <span>Your session ended. Please log in again.</span>
        </p>
      )}
      {demo && !staff && (
        <p className="notice" role="note">
          <InfoIcon />
          <span>Demo mode: create an account to try the app. Accounts are kept only until the app restarts.</span>
        </p>
      )}
      <div className="panel auth-card">
        <AuthForm mode="login" action={loginAction} next={next} signUp={!staff} />
      </div>
      {staff && (
        <p className="auth-switch">
          New to SwiftMed? <Link href={PHARMACY_SIGNUP}>Register your pharmacy</Link>
        </p>
      )}
    </div>
  );
}
