import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { registerAction } from "@/app/actions";
import { AuthForm } from "@/components/AuthForm";
import { getSession } from "@/lib/auth";
import { safeNext } from "@/lib/session";

export const metadata: Metadata = { title: "Create an account" };

type SearchParamsIn = Promise<Record<string, string | string[] | undefined>>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export default async function RegisterPage({ searchParams }: { searchParams: SearchParamsIn }) {
  const next = safeNext(first((await searchParams).next));
  if (await getSession()) redirect(next);

  return (
    <div className="page narrow auth-page">
      <h1>Create an account</h1>
      <p className="lead">You need an account to reserve medicine, so the pharmacy knows the hold is yours.</p>
      <div className="panel auth-card">
        <AuthForm mode="register" action={registerAction} next={next} />
      </div>
    </div>
  );
}
