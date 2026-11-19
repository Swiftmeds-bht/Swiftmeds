import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { loginAction } from "@/app/actions";
import { BrandMark } from "@/components/Icons";
import { Notices, queryText, type SearchParams } from "@/components/Notices";
import { getSession } from "@/lib/session";

export const metadata: Metadata = { title: "Sign in" };

export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  if (await getSession()) redirect("/");
  const params = await searchParams;

  return (
    <div className="page auth-page">
      <section className="panel auth-card" aria-labelledby="login-title">
        <span className="auth-mark">
          <BrandMark size={40} />
        </span>
        <h1 id="login-title">Sign in to your pharmacy</h1>
        <p className="muted">Manage your stock and hand over medicines that patients have reserved.</p>

        <Notices error={queryText(params.error)} notice={queryText(params.notice)} />

        <form action={loginAction} className="auth-form">
          <div className="form-field">
            <label htmlFor="email">Work email</label>
            <input id="email" name="email" type="email" autoComplete="username" required />
          </div>
          <div className="form-field">
            <label htmlFor="password">Password</label>
            <input id="password" name="password" type="password" autoComplete="current-password" required />
          </div>
          <button className="button" type="submit">
            Sign in
          </button>
        </form>

        <p className="field-hint">Use an account with pharmacy staff access.</p>
      </section>
    </div>
  );
}
