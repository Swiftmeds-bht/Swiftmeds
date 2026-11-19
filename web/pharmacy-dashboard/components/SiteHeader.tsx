import Link from "next/link";

import { activeHolds, currentSession, nameHolds, pharmacyLabel } from "@/lib/data";
import type { DashboardSession } from "@/lib/session";

import { AccountMenu } from "./AccountMenu";
import { HoldsBell } from "./HoldsBell";
import { BrandMark } from "./Icons";
import { MainNav } from "./MainNav";

/**
 * The dashboard header (SCRUM-138), built like the patient app's: the SwiftMed
 * mark, then, signed in, the sections, the holds bell and the account menu.
 */
export async function SiteHeader() {
  const session = await currentSession();
  return (
    <header className="site-header">
      <div className="wrap header-inner">
        <div className="header-start">
          <Link href="/" className="brand">
            <span className="brand-mark">
              <BrandMark />
            </span>
            <span className="brand-name">SwiftMed</span>
          </Link>
          <span className="header-tagline">Pharmacy</span>
        </div>
        {session ? <SignedInNav session={session} /> : null}
      </div>
    </header>
  );
}

async function SignedInNav({ session }: { session: DashboardSession }) {
  // The header never fails the page: a failed holds call shows in the bell.
  const [pharmacy, holds] = await Promise.all([
    pharmacyLabel(session),
    activeHolds(session).then(nameHolds, () => undefined),
  ]);
  const waiting = (holds ?? []).map((h) => ({
    holdId: h.holdId,
    medicine: h.medicine.name,
    quantity: h.quantity ?? 0,
    timeLeft: h.timeLeft,
  }));

  return (
    <div className="header-nav">
      <MainNav />
      <div className="header-actions">
        <HoldsBell holds={waiting} failed={holds === undefined} />
        <AccountMenu pharmacy={pharmacy} email={session.email} />
      </div>
    </div>
  );
}
