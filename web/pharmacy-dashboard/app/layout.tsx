import type { Metadata, Viewport } from "next";
import { Figtree } from "next/font/google";
import type { ReactNode } from "react";

import { SiteHeader } from "@/components/SiteHeader";

import "./globals.css";

// The patient app's typeface (SCRUM-138), downloaded at build time and served
// with the app, so pages never call Google.
const figtree = Figtree({ subsets: ["latin"], display: "swap", variable: "--font-sans" });

export const metadata: Metadata = {
  title: { default: "Pharmacy · SwiftMed", template: "%s · SwiftMed Pharmacy" },
  description: "Manage your pharmacy's stock and patient pickups.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#0b3b37",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en" className={figtree.variable}>
      <body>
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <SiteHeader />
        <main id="main">{children}</main>
        <footer className="site-footer">
          <div className="wrap footer-inner">
            <p className="footer-brand">SwiftMed Pharmacy</p>
            <p className="muted">Pickup codes are entered at the counter and never shown here.</p>
          </div>
        </footer>
      </body>
    </html>
  );
}
