import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import Link from "next/link";

import { AccountNav } from "@/components/AccountNav";

import "./globals.css";

// Downloaded at build time and served with the app, so pages never call Google.
const inter = Inter({ subsets: ["latin"], display: "swap", variable: "--font-sans" });

export const metadata: Metadata = {
  title: { default: "SwiftMed – find your medicine nearby", template: "%s · SwiftMed" },
  description: "See which nearby pharmacies in Bhutan have your medicine in stock.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={inter.variable}>
      <body>
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <header className="site-header">
          <div className="container">
            <Link href="/" className="brand">
              Swift<span className="brand-accent">Med</span>
            </Link>
            <div className="header-end">
              <span className="header-tagline">Medicine finder for Bhutan</span>
              <AccountNav />
            </div>
          </div>
        </header>
        <main id="main" className="container">
          {children}
        </main>
        <footer className="site-footer">
          <div className="container">
            <p className="muted">SwiftMed shows availability only. It does not give medical advice.</p>
          </div>
        </footer>
      </body>
    </html>
  );
}
