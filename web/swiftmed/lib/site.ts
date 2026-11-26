// The site's public address, for absolute URLs in link previews (SCRUM-157).
// Server-side only: read at render time, so one build works anywhere.

type Env = Readonly<Record<string, string | undefined>>;

const withScheme = (host: string) => (/^https?:\/\//.test(host) ? host : `https://${host}`);

/**
 * SITE_URL if set; on Vercel, the production address (stable across deploys,
 * unlike VERCEL_URL, which names one deployment); otherwise localhost.
 */
export function siteUrl(env: Env = process.env): string {
  const candidate =
    env.SITE_URL?.trim() || env.VERCEL_PROJECT_PRODUCTION_URL?.trim() || env.VERCEL_URL?.trim() || "http://localhost:3000";
  try {
    return new URL(withScheme(candidate)).origin;
  } catch {
    return "http://localhost:3000";
  }
}

export const SITE_NAME = "SwiftMed";
export const SITE_TITLE = "SwiftMed: find your medicine nearby";
export const SITE_DESCRIPTION =
  "See which nearby pharmacies in Bhutan have your medicine in stock, reserve it, and collect it with a pickup code.";
