import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { PharmacyStatusPanel } from "@/components/portal/PharmacyStatusPanel";
import { getSession } from "@/lib/auth";
import { VERIFIED, pharmacyOnce } from "@/lib/pharmacies";
import { ADMIN_PATH, PHARMACY_SIGNUP, PORTAL_PATH, isAdmin, isStaffAccount } from "@/lib/roles";

import "./portal.css";

export const metadata: Metadata = { title: { default: "Pharmacy portal", template: "%s · Pharmacy portal" } };

/**
 * The pharmacy portal (SCRUM-139): the former pharmacy dashboard, now part of
 * the one SwiftMed app. Staff only: signed out, people sign in and come back
 * here; patients see where to go instead; administrators go to /admin.
 *
 * Since SCRUM-142 a pharmacy signs itself up, so the portal opens only once an
 * administrator has approved the pharmacy; before that it shows where the
 * review stands. The gateway checks the role again on every call.
 */
export default async function PortalLayout({ children }: { children: ReactNode }) {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(PORTAL_PATH)}`);
  if (isAdmin(session)) redirect(ADMIN_PATH);

  if (!isStaffAccount(session)) {
    return (
      <div className="page narrow">
        <h1>For pharmacy staff</h1>
        <p className="lead">
          The pharmacy portal is where pharmacies update their stock and hand over reserved medicine. This is a patient account.
        </p>
        <div className="detail-actions">
          <Link href={PHARMACY_SIGNUP} className="button">
            Register your pharmacy
          </Link>
          <Link href="/" className="button button-outline">
            Search for a medicine
          </Link>
        </div>
      </div>
    );
  }
  // A staff account without its pharmacy yet: finish the sign-up.
  if (!session.pharmacyId) redirect(PHARMACY_SIGNUP);

  // If the registry cannot be reached, keep the portal open as before: the
  // gateway still refuses anything the pharmacy may not do.
  const pharmacy = await pharmacyOnce(session.pharmacyId);
  if (pharmacy && pharmacy.status !== VERIFIED) {
    return (
      <div className="page narrow">
        <PharmacyStatusPanel pharmacy={pharmacy} />
      </div>
    );
  }

  return <div className="portal">{children}</div>;
}
