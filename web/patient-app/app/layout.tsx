import type { Metadata, Viewport } from "next";
import Link from "next/link";

import "./globals.css";

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
    <html lang="en">
      <body>
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <header className="site-header">
          <Link href="/" className="brand">
            SwiftMed
          </Link>
        </header>
        <main id="main" className="container">
          {children}
        </main>
        <footer className="site-footer container">
          <p className="muted">SwiftMed shows availability only. It does not give medical advice.</p>
        </footer>
      </body>
    </html>
  );
}
