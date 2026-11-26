import type { Metadata, Viewport } from "next";
import { Figtree } from "next/font/google";
import Link from "next/link";

import { AccountNav, SignedInAlerts } from "@/components/AccountNav";
import { BrandMark } from "@/components/Icons";
import { SITE_DESCRIPTION, SITE_NAME, SITE_TITLE, siteUrl } from "@/lib/site";

import "./globals.css";

// Downloaded at build time and served with the app, so pages never call Google.
const figtree = Figtree({ subsets: ["latin"], display: "swap", variable: "--font-sans" });

// Link previews (SCRUM-157): absolute URLs need the site's address; the image
// is app/opengraph-image.tsx. Pages inherit all of this and set their own title.
export function generateMetadata(): Metadata {
  return {
    metadataBase: new URL(siteUrl()),
    title: { default: SITE_TITLE, template: `%s · ${SITE_NAME}` },
    description: SITE_DESCRIPTION,
    applicationName: SITE_NAME,
    openGraph: { type: "website", siteName: SITE_NAME, title: SITE_TITLE, description: SITE_DESCRIPTION, url: "/", locale: "en_GB" },
    twitter: { card: "summary_large_image", title: SITE_TITLE, description: SITE_DESCRIPTION },
  };
}

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#0b3b37",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={figtree.variable}>
      <body>
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <header className="site-header">
          <div className="wrap header-inner">
            <div className="header-start">
              <Link href="/" className="brand">
                <span className="brand-mark">
                  <BrandMark />
                </span>
                <span className="brand-name">SwiftMed</span>
              </Link>
            </div>
            <AccountNav />
          </div>
        </header>
        <main id="main">{children}</main>
        <footer className="site-footer">
          <div className="wrap footer-inner">
            <div className="footer-about">
              <p className="footer-brand">SwiftMed</p>
              <p className="muted">Find which pharmacies near you in Bhutan have your medicine, reserve it, and collect it with a code.</p>
            </div>
            <nav className="footer-col" aria-labelledby="footer-patients">
              <h2 id="footer-patients">For patients</h2>
              <ul>
                <li><Link href="/">Search medicines</Link></li>
                <li><Link href="/holds">My holds</Link></li>
                <li><Link href="/watchlist">Watchlist</Link></li>
              </ul>
            </nav>
            <nav className="footer-col" aria-labelledby="footer-pharmacies">
              <h2 id="footer-pharmacies">For pharmacies</h2>
              <ul>
                <li><Link href="/register/pharmacy">Register your pharmacy</Link></li>
                <li><Link href="/pharmacy-portal">Pharmacy portal</Link></li>
              </ul>
            </nav>
            <p className="footer-legal">SwiftMed shows availability only. It does not give medical advice.</p>
          </div>
        </footer>
        <SignedInAlerts />
      </body>
    </html>
  );
}
