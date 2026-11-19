"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

interface NavItem {
  href: string;
  label: string;
  /** Paths where this item is the current one. */
  matches: (path: string) => boolean;
  signedInOnly?: boolean;
}

const under = (base: string) => (path: string) => path === base || path.startsWith(`${base}/`);

// Search covers the steps of finding a medicine: results, a pharmacy, reserving.
export const NAV_ITEMS: NavItem[] = [
  { href: "/", label: "Search", matches: (p) => p === "/" || ["/results", "/pharmacy", "/reserve"].some((b) => under(b)(p)) },
  { href: "/holds", label: "My holds", matches: under("/holds"), signedInOnly: true },
  { href: "/watchlist", label: "Watchlist", matches: under("/watchlist"), signedInOnly: true },
];

/** The main sections (SCRUM-137), with the current one marked for everyone, screen readers included. */
export function MainNav({ signedIn }: { signedIn: boolean }) {
  const path = usePathname() ?? "/";
  return (
    <nav aria-label="Main" className="main-nav">
      <ul>
        {NAV_ITEMS.filter((item) => signedIn || !item.signedInOnly).map((item) => (
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
