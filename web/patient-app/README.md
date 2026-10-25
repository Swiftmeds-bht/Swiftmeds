# Patient web app

Next.js app where patients search for a medicine and see which nearby pharmacies
have it in stock. Owner: Pema. Responsive: works on phones and desktops.

## Pages

| Route | What it shows |
|---|---|
| `/` | Search form: medicine name, town, radius |
| `/results?q=…&place=…&radius=…` | Matching medicines with in-stock pharmacies (distance, price, stock, freshness), and same-molecule alternatives when nothing is in stock |
| `/pharmacy/[id]` | Pharmacy detail (placeholder: address and map link; reservations come later) |

The search form is a plain `GET` form, so it works without JavaScript and every
search is a shareable URL.

## Data

`lib/api.ts` calls the gateway at `GATEWAY_URL` (`GET /api/v1/search`). Until the
gateway exposes search (SCRUM-49), or when it is unreachable, it falls back to
demo data from `lib/mock-data.ts` (built from `deploy/seed/`), and the results
page says "Showing demo data".

## Run

Requires Node.js 20+.

```bash
npm ci
npm run dev                                   # http://localhost:3000 (demo data)
GATEWAY_URL=http://localhost:8080 npm run dev # use the gateway
```

## Checks

```bash
npm run lint        # ESLint with eslint-config-next (core web vitals + TypeScript)
npm run typecheck   # tsc --noEmit
npm test            # Vitest: formatting, API client, axe accessibility, WCAG contrast
npm run build       # production build
```

Accessibility: `components/a11y.test.tsx` runs axe-core (WCAG 2.1 A/AA) on the
search form and results and fails on serious or critical violations. axe cannot
measure colour contrast in jsdom, so `lib/contrast.test.ts` checks every colour
token in `app/globals.css` against the WCAG contrast ratios instead.
