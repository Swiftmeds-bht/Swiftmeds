# Patient web app

Next.js app where patients search for a medicine and see which nearby pharmacies
have it in stock. Owner: Pema. Responsive: works on phones and desktops.

## Pages

| Route | What it shows |
|---|---|
| `/` | Search form: medicine name (with suggestions), location, radius |
| `/results?q=…&place=…&radius=…` | Matching medicines with in-stock pharmacies (best match first: distance, price, stock, freshness badge), and same-molecule alternatives when nothing is in stock |
| `/results?q=…&lat=…&lng=…&radius=…` | The same, searched from the patient's device location |
| `/pharmacy/[id]` | Pharmacy detail (placeholder: address and map link; reservations come later) |

The search form is a plain `GET` form, so it works without JavaScript and every
search is a shareable URL. With JavaScript it also shows suggestions, offers
"Use my location", and shows a "Searching…" state.

## Search box and location

- **Suggestions**: after 2 characters and a 250 ms pause the box asks
  `/api/suggest`, a route inside this app that asks the gateway
  (`GET /api/v1/search/autocomplete?prefix=…&limit=8`). The patient can pick with
  the mouse or the keyboard (arrows, Enter, Escape). If suggestions fail, the box
  still works: the patient types the whole name.
- **Location**: "Use my location" asks the browser, only when pressed. The
  coordinates are rounded to 3 decimals (about 110 m) because they appear in the
  page address. If the patient says no, or the browser cannot share a location,
  a message says so and the town list ("Near") is the fallback. The town list also
  works without JavaScript.

## Loading, empty and error states

| State | What the patient sees |
|---|---|
| Loading | "Searching nearby pharmacies…" with grey placeholder cards (`ResultsSkeleton`, also `app/results/loading.tsx`) |
| Empty: no query | "Type a medicine name to search." |
| Empty: no match | "No medicines match …" |
| Empty: nobody has it | "No pharmacy nearby has … in stock." plus substitutes when the gateway sends them |
| Error | A clear message and a Try again link (`SearchProblem`): cannot reach the search, too slow, not working, not available yet, or the search was not accepted |
| Some details missing | A note when the gateway reports optional services as down (`x-search-degraded` header) |

## Data

`lib/api.ts` calls the gateway at `GATEWAY_URL`:

| Route | Used for |
|---|---|
| `GET /api/v1/search?q=…&lat=…&lng=…&radius_m=…` | Results (`SearchMedicine`) |
| `GET /api/v1/search/autocomplete?prefix=…&limit=…` | Suggestions (`Autocomplete`) |

JSON follows `proto/search/v1/search.proto` in protobuf JSON form (camelCase;
int64 values such as money arrive as strings).

- With `GATEWAY_URL` set, a failing gateway is shown as an error. The app never
  replaces a failed search with demo stock.
- With `GATEWAY_URL` empty, the app uses demo data from `lib/mock-data.ts` (built
  from `deploy/seed/`), and the results page says "Showing demo data".

## Run

Requires Node.js 20+.

```bash
npm ci
npm run dev                                   # http://localhost:3000 (demo data)
GATEWAY_URL=http://localhost:8080 npm run dev # use the gateway
```

To try every state with real network calls before the gateway has the search
routes, run the stand-in gateway in another terminal:

```bash
npm run dev:gateway                           # http://localhost:8080
GATEWAY_URL=http://localhost:8080 npm run dev
```

Then search for `napa` (normal), `slow` (loading), `error` (503), `gone` (404),
`hang` (timeout), `degraded`, `none` or `nostock`. See `scripts/dev-gateway.mjs`.

## Checks

```bash
npm run lint        # ESLint with eslint-config-next (core web vitals + TypeScript)
npm run typecheck   # tsc --noEmit
npm test            # Vitest: formatting, API client, components, axe accessibility, WCAG contrast
npm run build       # production build
```

Accessibility: `components/a11y.test.tsx` and `components/MedicineCombobox.a11y.test.tsx`
run axe-core (WCAG 2.1 A/AA) on the search form (including the open suggestion
list and the location messages), the results, and the loading and error states, and
fail on serious or critical violations. axe cannot measure colour contrast in
jsdom, so `lib/contrast.test.ts` checks every colour token in `app/globals.css`
against the WCAG contrast ratios instead.

Mobile: the layout is one column below 40 rem. Buttons and fields are at least
2.75 rem (44 px) tall, text wraps instead of overflowing, and the page sets the
viewport meta tag. Check it by hand at 320 and 360 px wide.
