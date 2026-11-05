import Link from "next/link";

import { logoutAction } from "@/app/actions";
import { getSession } from "@/lib/auth";

// Header links: Sign in, or My holds and Sign out. Reads the session cookie on the server.
export async function AccountNav() {
  const session = await getSession();
  return (
    <nav aria-label="Account" className="account-nav">
      {session ? (
        <>
          <Link href="/holds" className="nav-link">
            My holds
          </Link>
          <form action={logoutAction}>
            <button type="submit" className="link-button nav-link">
              Sign out
            </button>
          </form>
        </>
      ) : (
        <Link href="/login" className="nav-link">
          Sign in
        </Link>
      )}
    </nav>
  );
}
