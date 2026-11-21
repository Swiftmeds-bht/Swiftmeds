import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import type { ReactNode } from "react";

import { getSession } from "@/lib/auth";
import { ADMIN_PATH, isAdmin } from "@/lib/roles";

export const metadata: Metadata = {
  title: { default: "Admin", template: "%s · SwiftMed admin" },
  robots: { index: false, follow: false },
};

/**
 * SwiftMed's admin pages (SCRUM-142). Reached through the normal login; nothing
 * public links here. Signed out, people log in and come back; any other
 * account gets "not found", so the page does not advertise itself. The gateway
 * checks the admin role again on every call.
 */
export default async function AdminLayout({ children }: { children: ReactNode }) {
  const session = await getSession();
  if (!session) redirect(`/login?next=${encodeURIComponent(ADMIN_PATH)}`);
  if (!isAdmin(session)) notFound();
  return <div className="admin">{children}</div>;
}
