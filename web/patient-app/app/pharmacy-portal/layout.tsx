import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { getSession } from "@/lib/auth";
import { isPharmacyStaff, PORTAL_PATH } from "@/lib/roles";

import "./portal.css";

export const metadata: Metadata = { title: { default: "Pharmacy portal", template: "%s · Pharmacy portal" } };

/**
 * The pharmacy portal (SCRUM-139): the former pharmacy dashboard, now part of
 * the one SwiftMed app. Staff only: signed out, people sign in and come back
 * here; patients see where to go instead. The gateway checks the role again on
 * every call.
 */
export default async function PortalLayout({ children }: { children: ReactNode }) {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(PORTAL_PATH)}`);

  if (!isPharmacyStaff(session)) {
    return (
      <div className="page narrow">
        <h1>For pharmacy staff</h1>
        <p className="lead">
          The pharmacy portal is where pharmacies update their stock and hand over reserved medicine. You are signed in as a patient.
        </p>
        <p>
          <Link href="/" className="button">
            Search for a medicine
          </Link>
        </p>
      </div>
    );
  }

  return <div className="portal">{children}</div>;
}
