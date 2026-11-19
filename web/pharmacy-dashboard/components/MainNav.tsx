"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const under = (base: string) => (path: string) => path === base || path.startsWith(`${base}/`);

export const NAV_ITEMS = [
  { href: "/", label: "Overview", matches: (p: string) => p === "/" },
  { href: "/holds", label: "Holds", matches: under("/holds") },
  { href: "/stock", label: "Stock", matches: under("/stock") },
  { href: "/upload", label: "Upload CSV", matches: under("/upload") },
];

/** The dashboard's sections (SCRUM-138), the current one marked for everyone. */
export function MainNav() {
  const path = usePathname() ?? "/";
  return (
    <nav aria-label="Main" className="main-nav">
      <ul>
        {NAV_ITEMS.map((item) => (
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
