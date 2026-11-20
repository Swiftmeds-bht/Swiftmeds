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
import { VERIFIED, pharmacyOnce } from "@/lib/pharmacies";
import { isAdmin, isStaffAccount } from "@/lib/roles";
import type { Session } from "@/lib/types";

import { AccountMenu } from "./AccountMenu";
import { AlertListener } from "./AlertListener";
import { LoginMenu } from "./LoginMenu";
import { MainNav } from "./MainNav";
import { NotificationBell } from "./NotificationBell";
import { SignUpMenu } from "./SignUpMenu";
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
  // Until an administrator approves the pharmacy, the portal only shows where
  // its review stands (SCRUM-142): no sections, no bell.
  const pharmacy = session.pharmacyId ? await pharmacyOnce(session.pharmacyId) : undefined;
  const approved = Boolean(session.pharmacyId) && (pharmacy === undefined || pharmacy.status === VERIFIED);
  // The header never fails the page: a failed holds call shows in the bell.
  const [label, holds, profile] = await Promise.all([
    portal ? pharmacyLabel(portal) : Promise.resolve(pharmacy?.name || "Your pharmacy"),
    portal && approved ? activeHolds(portal).then((h) => nameHolds(h), () => undefined) : Promise.resolve(undefined),
    loadProfile(session),
  ]);
  const waiting = (holds ?? []).map((h) => ({ holdId: h.holdId, medicine: h.medicine.name, quantity: h.quantity ?? 0, timeLeft: h.timeLeft }));
  const name = profile?.displayName || undefined;
  const menu = <AccountMenu email={session.email} name={name} initial={initialOf(name, session.email)} pharmacy={label} />;
  if (!approved) {
    return (
      <div className="header-nav">
        <div className="header-actions">{menu}</div>
      </div>
    );
  }
  return (
    <div className="header-nav">
      <MainNav audience="staff" />
      <span className="header-divider" aria-hidden="true" />
      <div className="header-actions">
        <HoldsBell holds={waiting} failed={holds === undefined} />
        {menu}
      </div>
    </div>
  );
}

/** The header for SwiftMed administrators (SCRUM-142): the review page and the account menu. */
async function AdminNav({ session }: { session: Session }) {
  const profile = await loadProfile(session);
  const name = profile?.displayName || undefined;
  return (
    <div className="header-nav">
      <MainNav audience="admin" />
      <span className="header-divider" aria-hidden="true" />
      <div className="header-actions">
        <AccountMenu email={session.email} name={name} initial={initialOf(name, session.email)} note="SwiftMed administrator" />
      </div>
    </div>
  );
}

/**
 * Header navigation (SCRUM-137, by role since SCRUM-139). Patients: the main
 * sections, the notification bell and the account menu. Pharmacy staff: the
 * portal's sections instead; administrators, the review page. Signed out:
 * Search, For pharmacies, and the Log in and Sign up menus (SCRUM-140, SCRUM-142). Reads the session
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
          <SignUpMenu />
        </div>
      </div>
    );
  }
  if (isAdmin(session)) return <AdminNav session={session} />;
  if (isStaffAccount(session)) return <StaffNav session={session} />;

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
  return session && !isStaffAccount(session) && !isAdmin(session) ? <AlertListener /> : null;
}
