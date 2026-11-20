import type { Metadata, Viewport } from "next";
import { Figtree } from "next/font/google";
import Link from "next/link";

import { AccountNav, SignedInAlerts } from "@/components/AccountNav";
import { BrandMark } from "@/components/Icons";

import "./globals.css";

// Downloaded at build time and served with the app, so pages never call Google.
const figtree = Figtree({ subsets: ["latin"], display: "swap", variable: "--font-sans" });

export const metadata: Metadata = {
  title: { default: "SwiftMed: find your medicine nearby", template: "%s · SwiftMed" },
  description: "See which nearby pharmacies in Bhutan have your medicine in stock.",
};

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
                <li><Link href="/pharmacy-portal">Pharmacy portal</Link></li>
                <li><Link href="/pharmacy-portal/upload">Upload stock</Link></li>
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
