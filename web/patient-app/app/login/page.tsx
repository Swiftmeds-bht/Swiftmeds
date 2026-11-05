import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { loginAction } from "@/app/actions";
import { AuthForm } from "@/components/AuthForm";
import { InfoIcon } from "@/components/Icons";
import { getSession } from "@/lib/auth";
import { DEMO_ACCOUNT } from "@/lib/demo-holds";
import { holdsBackend } from "@/lib/holds";
import { safeNext } from "@/lib/session";

export const metadata: Metadata = { title: "Sign in" };

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export default async function LoginPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const sp = await searchParams;
  const next = safeNext(first(sp.next));
  if (await getSession()) redirect(next);
  const demo = holdsBackend().source === "demo";

  return (
    <div className="page auth-page">
      <h1>Sign in</h1>
      <p className="lead">Sign in to reserve medicine and see your holds.</p>
      {first(sp.expired) && (
        <p className="notice notice-warning" role="note">
          <InfoIcon />
          <span>Your session ended. Please sign in again.</span>
        </p>
      )}
      {demo && (
        <p className="notice" role="note">
          <InfoIcon />
          <span>
            Demo mode: try <strong>{DEMO_ACCOUNT.email}</strong> with password <strong>{DEMO_ACCOUNT.password}</strong>, or create
            an account. Accounts are kept only until the app restarts.
          </span>
        </p>
      )}
      <div className="card auth-card">
        <AuthForm mode="login" action={loginAction} next={next} />
      </div>
    </div>
  );
}
