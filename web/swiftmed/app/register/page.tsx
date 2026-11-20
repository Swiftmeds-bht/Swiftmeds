import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { registerAction } from "@/app/actions";
import { AudienceSwitch } from "@/components/AudienceSwitch";
import { AuthForm } from "@/components/AuthForm";
import { getSession } from "@/lib/auth";
import { PATIENT_SIGNUP, PHARMACY_SIGNUP, homeAfterSignIn } from "@/lib/roles";
import { safeNext } from "@/lib/session";

export const metadata: Metadata = { title: "Sign up" };

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export default async function RegisterPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const next = safeNext(first((await searchParams).next));
  const session = await getSession();
  if (session) redirect(homeAfterSignIn(session, next));

  return (
    <div className="page narrow auth-page">
      <AudienceSwitch
        label="Sign up as"
        options={[
          { href: PATIENT_SIGNUP, label: "Patient", current: true },
          { href: PHARMACY_SIGNUP, label: "Pharmacy", current: false },
        ]}
      />
      <h1>Create an account</h1>
      <p className="lead">You need an account to reserve medicine, so the pharmacy knows the hold is yours.</p>
      <div className="panel auth-card">
        <AuthForm mode="register" action={registerAction} next={next} />
      </div>
    </div>
  );
}
