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
import { activeHolds, nameHolds, pharmacyLabel, portalSession } from "@/lib/portal/data";
import { initialOf, profileBackend, type Profile } from "@/lib/profile";
import { isPharmacyStaff } from "@/lib/roles";
import type { Session } from "@/lib/types";

import { AccountMenu } from "./AccountMenu";
import { AlertListener } from "./AlertListener";
import { LoginMenu } from "./LoginMenu";
import { MainNav } from "./MainNav";
import { NotificationBell } from "./NotificationBell";
import { HoldsBell } from "./portal/HoldsBell";

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

/** The header for pharmacy staff (SCRUM-139): the portal's sections, the holds bell, the account menu. */
async function StaffNav({ session }: { session: Session }) {
  const portal = await portalSession();
  // The header never fails the page: a failed holds call shows in the bell.
  const [pharmacy, holds, profile] = await Promise.all([
    portal ? pharmacyLabel(portal) : Promise.resolve("Your pharmacy"),
    portal ? activeHolds(portal).then((h) => nameHolds(h), () => undefined) : Promise.resolve(undefined),
    loadProfile(session),
  ]);
  const waiting = (holds ?? []).map((h) => ({ holdId: h.holdId, medicine: h.medicine.name, quantity: h.quantity ?? 0, timeLeft: h.timeLeft }));
  const name = profile?.displayName || undefined;
  return (
    <div className="header-nav">
      <MainNav audience="staff" />
      <span className="header-divider" aria-hidden="true" />
      <div className="header-actions">
        <HoldsBell holds={waiting} failed={holds === undefined} />
        <AccountMenu email={session.email} name={name} initial={initialOf(name, session.email)} pharmacy={pharmacy} />
      </div>
    </div>
  );
}

/**
 * Header navigation (SCRUM-137, by role since SCRUM-139). Patients: the main
 * sections, the notification bell and the account menu. Pharmacy staff: the
 * portal's sections instead. Signed out: Search, For pharmacies, the Log in
 * menu (patients or pharmacy staff, SCRUM-140) and Sign up. Reads the session
 * cookie on the server, so tokens never
 * reach the browser.
 */
export async function AccountNav() {
  const session = await getSession();
  if (!session) {
    return (
      <div className="header-nav">
        <MainNav audience="guest" />
        <span className="header-divider" aria-hidden="true" />
        <div className="header-actions">
          <LoginMenu />
          <Link href="/register" className="button button-small header-cta">
            Sign up
          </Link>
        </div>
      </div>
    );
  }
  if (isPharmacyStaff(session)) return <StaffNav session={session} />;

  const [notifications, profile, jar] = await Promise.all([loadNotifications(session), loadProfile(session), cookies()]);
  const list = notifications ?? [];
  // Back-in-stock notifications name a medicine and a pharmacy (SCRUM-138).
  const book = await nameBook(list.map((n) => ({ productId: n.params.product_id, pharmacyId: n.params.pharmacy_id })));
  const seenAt = parseSeenAt(jar.get(NOTIFICATIONS_SEEN_COOKIE)?.value);
  const name = profile?.displayName || undefined;

  return (
    <div className="header-nav">
      <MainNav audience="patient" />
      <span className="header-divider" aria-hidden="true" />
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
  const session = await getSession();
  // Back-in-stock alerts are for patients; staff have the holds bell.
  return session && !isPharmacyStaff(session) ? <AlertListener /> : null;
}
