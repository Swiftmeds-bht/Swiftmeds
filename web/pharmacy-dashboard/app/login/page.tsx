import { redirect } from "next/navigation";

import { loginAction } from "@/app/actions";
import { getSession } from "@/lib/session";

type LoginPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

function queryText(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" ? value : undefined;
}

export default async function LoginPage({ searchParams }: LoginPageProps) {
  if (await getSession()) redirect("/");

  const params = await searchParams;
  const error = queryText(params.error);
  const notice = queryText(params.notice);

  return (
    <main className="auth-page">
      <section className="auth-card" aria-labelledby="login-title">
        <div className="brand-mark" aria-hidden="true">
          SM
        </div>

        <p className="eyebrow">SwiftMeds · Pharmacy</p>
        <h1 id="login-title">Sign in to your dashboard</h1>
        <p className="auth-description">
          Manage your pharmacy stock and handle active medicine holds.
        </p>

        {error ? (
          <p className="message message-error" role="alert">
            {error}
          </p>
        ) : null}

        {notice ? (
          <p className="message message-success" role="status">
            {notice}
          </p>
        ) : null}

        <form action={loginAction} className="auth-form">
          <label htmlFor="email">Work email</label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            required
          />

          <label htmlFor="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
          />

          <button className="button button-primary" type="submit">
            Sign in
          </button>
        </form>

        <p className="auth-footnote">
          Sign in with an account that has pharmacy staff access.
        </p>
      </section>
    </main>
  );
}