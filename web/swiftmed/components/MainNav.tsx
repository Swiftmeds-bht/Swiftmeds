"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { ADMIN_PATH, PHARMACY_SIGNUP, PORTAL_PATH } from "@/lib/roles";

/** Who is looking: the header shows different sections to each (SCRUM-139, admin SCRUM-142). */
export type NavAudience = "guest" | "patient" | "staff" | "admin";

interface NavItem {
  href: string;
  label: string;
  /** Paths where this item is the current one. */
  matches: (path: string) => boolean;
}

const under = (base: string) => (path: string) => path === base || path.startsWith(`${base}/`);

// Search covers the steps of finding a medicine: results, a pharmacy, reserving.
const SEARCH: NavItem = {
  href: "/",
  label: "Search",
  matches: (p) => p === "/" || ["/results", "/pharmacy", "/reserve"].some((b) => under(b)(p)),
};

export const NAV_ITEMS: Record<NavAudience, NavItem[]> = {
  // "For pharmacies" leads to sign-up, where registered pharmacies find the login too (SCRUM-142).
  guest: [SEARCH, { href: PHARMACY_SIGNUP, label: "For pharmacies", matches: under(PHARMACY_SIGNUP) }],
  patient: [
    SEARCH,
    { href: "/holds", label: "My holds", matches: under("/holds") },
    { href: "/watchlist", label: "Watchlist", matches: under("/watchlist") },
  ],
  // The pharmacy portal's sections.
  staff: [
    { href: PORTAL_PATH, label: "Overview", matches: (p) => p === PORTAL_PATH },
    { href: `${PORTAL_PATH}/holds`, label: "Holds", matches: under(`${PORTAL_PATH}/holds`) },
    { href: `${PORTAL_PATH}/stock`, label: "Stock", matches: under(`${PORTAL_PATH}/stock`) },
    { href: `${PORTAL_PATH}/upload`, label: "Upload CSV", matches: under(`${PORTAL_PATH}/upload`) },
  ],
  // SwiftMed administrators (SCRUM-142).
  admin: [{ href: ADMIN_PATH, label: "Pharmacies", matches: under(ADMIN_PATH) }],
};

/** The main sections for this audience, with the current one marked for everyone, screen readers included. */
export function MainNav({ audience }: { audience: NavAudience }) {
  const path = usePathname() ?? "/";
  return (
    <nav aria-label="Main" className="main-nav">
      <ul>
        {NAV_ITEMS[audience].map((item) => (
          <li key={item.href}>
            <Link href={item.href} className="nav-link" aria-current={item.matches(path) ? "page" : undefined}>
              {item.label}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
