import Link from "next/link";
import { cookies } from "next/headers";

import { getSession } from "@/lib/auth";
import {
  NOTIFICATIONS_SEEN_COOKIE,
  describeNotification,
  newestCreatedAt,
  notificationsBackend,
  parseSeenAt,
  unreadCount,
  type AppNotification,
} from "@/lib/notifications";
import { nameBook } from "@/lib/names";
import { initialOf, profileBackend, type Profile } from "@/lib/profile";
import type { Session } from "@/lib/types";

import { AccountMenu } from "./AccountMenu";
import { AlertListener } from "./AlertListener";
import { MainNav } from "./MainNav";
import { NotificationBell } from "./NotificationBell";

// The header never fails the page: if a call fails, the bell says so and the
// account menu shows the email.
async function loadNotifications(session: Session): Promise<AppNotification[] | undefined> {
  try {
    return await notificationsBackend().list(session);
  } catch {
    return undefined;
  }
}

async function loadProfile(session: Session): Promise<Profile | undefined> {
  try {
    return await profileBackend().get(session);
  } catch {
    return undefined;
  }
}

/**
 * Header navigation (SCRUM-137): the main sections and, signed in, the
 * notification bell and the account menu; signed out, a Sign in link. Reads the
 * session cookie on the server, so tokens never reach the browser.
 */
export async function AccountNav() {
  const session = await getSession();
  if (!session) {
    return (
      <div className="header-nav">
        <MainNav signedIn={false} />
        <div className="header-actions">
          <Link href="/login" className="nav-link nav-link-boxed">
            Sign in
          </Link>
        </div>
      </div>
    );
  }

  const [notifications, profile, jar] = await Promise.all([loadNotifications(session), loadProfile(session), cookies()]);
  const list = notifications ?? [];
  // Back-in-stock notifications name a medicine and a pharmacy (SCRUM-138).
  const book = await nameBook(list.map((n) => ({ productId: n.params.product_id, pharmacyId: n.params.pharmacy_id })));
  const seenAt = parseSeenAt(jar.get(NOTIFICATIONS_SEEN_COOKIE)?.value);
  const name = profile?.displayName || undefined;

  return (
    <div className="header-nav">
      <MainNav signedIn />
      <div className="header-actions">
        <NotificationBell
          items={list.map((n) => describeNotification(n, book))}
          unread={unreadCount(list, seenAt)}
          newest={newestCreatedAt(list)}
          failed={notifications === undefined}
        />
        <AccountMenu email={session.email} name={name} initial={initialOf(name, session.email)} />
      </div>
    </div>
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
