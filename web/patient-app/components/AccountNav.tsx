import Link from "next/link";

import { logoutAction } from "@/app/actions";
import { getSession } from "@/lib/auth";

import { AlertListener } from "./AlertListener";

// Header links: Sign in, or My holds, Watchlist and Sign out. Reads the session cookie on the server.
export async function AccountNav() {
  const session = await getSession();
  return (
    <nav aria-label="Account" className="account-nav">
      {session ? (
        <>
          <Link href="/holds" className="nav-link">
            My holds
          </Link>
          <Link href="/watchlist" className="nav-link">
            Watchlist
          </Link>
          <form action={logoutAction}>
            <button type="submit" className="link-button nav-link">
              Sign out
            </button>
          </form>
        </>
      ) : (
        <Link href="/login" className="nav-link nav-link-boxed">
          Sign in
        </Link>
      )}
    </nav>
  );
}

/**
 * Back-in-stock alerts while signed in (SCRUM-71). The layout puts this at the end
 * of <body>, not in the header: the header's backdrop-filter would make the fixed
 * alerts position against the header instead of the window.
 */
export async function SignedInAlerts() {
  return (await getSession()) ? <AlertListener /> : null;
}
