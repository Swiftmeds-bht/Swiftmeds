# Patient web app

Next.js app where patients search for a medicine and see which nearby pharmacies
have it in stock. Owner: Pema. Responsive: works on phones and desktops.

## Pages

| Route | What it shows |
|---|---|
| `/` | Search form: medicine name (with suggestions), location, radius |
| `/results?q=…&place=…&radius=…` | Matching medicines with in-stock pharmacies (best match first and highlighted: distance, price, stock, and freshness as 1 to 3 bars with words), and same-molecule alternatives when nothing is in stock. The search sits in one compact bar; the town and radius fold into a "Near …, within … · Change" line (a native `<details>`, so it works without JavaScript) |
| `/results?q=…&lat=…&lng=…&radius=…` | The same, searched from the patient's device location |
| `/pharmacy/[id]` | Pharmacy detail: name, address, dzongkhag, phone, map link and a link back to search (opening hours and stock list come later). Live from the gateway, demo data (labelled) with `USE_DEMO_DATA=true`; see [Data](#data) |
| `/reserve?product=…&pharmacy=…` | Confirm a reservation: medicine, pharmacy, price, how many units (1 to 5, no more than in stock), and the total to pay at the counter, which updates with the number of units. Opened from Reserve on a result |
| `/holds/[id]` | One hold: "Checking stock" (refreshes every 2 s), then the pickup code on a ticket with a countdown, or why it was not held, expired or cancelled. When a hold has just turned active (status changed in the last 15 s), the code's digits rise into place once; reloads do not replay it, and reduced motion turns it off |
| `/holds` | My holds: current (with Cancel) and past |
| `/watchlist` | Medicines the patient asked to be told about: Waiting or Back in stock, with Remove |
| `/login`, `/register` | Sign in and create an account. Both return to the page that asked, e.g. the reservation |

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
| Empty: no match | "No medicines match …" with how to fix the search |
| Empty: nobody has it | "No pharmacy nearby has … in stock." plus substitutes when the gateway sends them |
| Error | A clear message and a Try again link (`SearchProblem`): cannot reach the search, too slow, not working, not available yet, or the search was not accepted |
| Some details missing | A note when the gateway reports optional services as down (`x-search-degraded` header). **Not shown yet:** the gateway does not forward that header from the Search service until SCRUM-124 lands. The app already reads it (`lib/api.ts`), and `npm run dev:gateway` with the search `degraded` shows the note |

## Reserving and collecting (SCRUM-60)

A patient presses **Reserve** on a result, signs in if needed, picks how many units
and confirms. The hold page follows the hold through its states
(`docs/api/openapi.yaml`, `HoldStatus`):

| Status | What the patient sees |
|---|---|
| Pending | "Checking stock" with a turning loader; the page refreshes every 2 seconds (the API is poll-based) |
| Active | The pickup code, large and read digit by digit by screen readers, a countdown to the expiry time (Bhutan time), and Cancel |
| Rejected | Why ("Only 1 left at this pharmacy", …), that nothing was reserved, and a link to search again |
| Expired | When it ran out, and a link to search again. The page turns expired by itself when the countdown ends |
| Cancelled, collected | A short final message |

**Session.** Signing in stores the access token, its expiry and the refresh token
in an httpOnly, SameSite=Lax cookie (`lib/session.ts`, `lib/auth.ts`), so page
scripts cannot read them. All gateway calls with the token happen on the server
(`app/actions.ts`, server components). The `next` address after signing in must
be a path on this site, so a crafted link cannot send a patient elsewhere.

**Staying signed in (SCRUM-131).** The access token lasts 15 minutes (`JWT_TTL`).
The app swaps the refresh token for a new pair (`POST /api/v1/auth/refresh`,
`lib/refresh.ts`) so the patient is not signed out:

- **Before it expires**: `proxy.ts` (Next.js 16 Proxy, formerly middleware) runs
  before every page, route handler and server action that carries the cookie. Within
  60 s of expiry, or after it, it refreshes, puts the new cookie on the forwarded
  request (so the page rendering that request already uses the new token) and on the
  response. It has to happen there: server components cannot set cookies.
- **After a 401**: server actions refresh once and retry the call
  (`withFreshToken`). A second 401 signs the patient out; there is no loop.
- **Refused refresh** (400/401, or any 4xx): the cookie is removed and pages that need
  a session send the patient to sign in. **Gateway down or 5xx**: nothing changes, and
  the next request tries again.
- **Concurrent requests**: refresh tokens work once, and a browser often sends
  several requests with the same cookie (a page, the alert stream, prefetches).
  `refreshOnce` shares one gateway call between them and gives requests that arrive
  up to 30 s later with the old cookie the same new tokens. This lives in the
  server's memory, so with several app instances use sticky sessions (or move it to
  a shared store).
- The cookie lasts 7 days when there is a refresh token (the gateway's own limit is
  `JWT_REFRESH_TTL`, 30 days). Signing out also revokes the refresh token
  (`POST /api/v1/auth/logout`, best effort).
- A server-component page that still gets a 401 sends the patient to sign in again
  (`/session-ended` clears the cookie) and back afterwards.

The gateway does not serve the auth routes yet (SCRUM-115), so this is covered by
unit tests with a mocked gateway (`lib/refresh.test.ts`, `lib/session-proxy.test.ts`)
and can only be tried live once SCRUM-115 lands. Until then a refresh is refused and
the patient signs in again after 15 minutes, as before.

**Gateway routes** (`lib/holds.ts`, `lib/refresh.ts`): `POST /api/v1/auth/register`, `POST /api/v1/auth/login`,
`POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout`,
`POST /api/v1/holds`, `GET /api/v1/holds/{id}`, `GET /api/v1/holds`,
`POST /api/v1/holds/{id}/cancel`. Most are still "planned" in the API spec; until
they exist, a gateway answering 404 shows "This is not available yet".

**Demo mode** (`USE_DEMO_DATA=true`, `lib/demo-holds.ts`): accounts and holds live in
the server's memory and are lost on restart. Sign in as `patient@demo.bt` /
`swiftmed-demo`, or create an account. A hold is pending for 2 seconds, then active
for 30 minutes. It is rejected when more units are asked for than are left, or when
the pharmacy had only its last unit (the demo pretends someone else got it first,
so the rejected screen can be seen: reserve "Demo Paracetamol Syrup" at Thimphu 01).

## Back-in-stock alerts (SCRUM-71)

When no pharmacy in the searched area has a medicine, its card offers **Notify me
when it is back**. That puts the medicine and the searched area (point and radius)
on the patient's watchlist (`POST /api/v1/watchlist`). While the patient is signed
in, every page listens for alerts, and one appears in the corner as soon as a
pharmacy in the area gets the medicine: no refresh needed.

```
browser EventSource ──> /api/alerts (this app) ──> gateway GET /api/v1/watchlist/alerts ──> Watchlist StreamAlerts
          (cookie)                (bearer token, server side)        (SSE)                        (gRPC stream)
```

- The browser cannot send the access token with `EventSource`, and must not see
  it, so `app/api/alerts/route.ts` reads the session cookie and calls the gateway
  itself (`lib/watchlist.ts`, `lib/sse.ts`).
- A comment line every 15 s keeps proxies from closing a quiet stream. When the
  browser leaves, the route aborts the gateway request, which ends the gRPC stream.
- `components/AlertListener.tsx` reconnects after a dropped connection (2 s, then
  doubling up to 60 s), and stops once the session has ended. Alerts that arrived
  while the patient was away are sent on the next connection.

**Demo mode** (`lib/watchlist.ts`, `DemoWatchlist`): a watched medicine "comes
back" 10 seconds after pressing Notify me, at a demo pharmacy that stocks it. To
see it, search "Demo Paracetamol 500" near Thimphu within 1 km, press Notify me,
and wait.

## Data

The app reads live data from the gateway by default. `lib/data-source.ts` is the
one place that decides:

| Setting | Data |
|---|---|
| nothing set | the gateway at `http://localhost:8080` |
| `GATEWAY_URL=http://gateway:8080` | the gateway at that address (Docker Compose sets this) |
| `USE_DEMO_DATA=true` | built-in demo data (`lib/mock-data.ts`, `lib/demo-holds.ts`), labelled on every screen |

Both are read by the server on each request, so one Docker image works anywhere.
`lib/api.ts` calls these gateway routes:

| Route | Used for |
|---|---|
| `GET /api/v1/search?q=…&lat=…&lng=…&radius_m=…` | Results (`SearchMedicine`) |
| `GET /api/v1/search/autocomplete?prefix=…&limit=…` | Suggestions (`Autocomplete`) |
| `GET /api/v1/pharmacies/{pharmacyId}` | The pharmacy page (`lookupPharmacy`; public in the spec) |

JSON follows `proto/search/v1/search.proto` in protobuf JSON form (camelCase;
int64 values such as money arrive as strings).

**Pharmacy page.** In gateway mode it reads `GET /api/v1/pharmacies/{pharmacyId}`
and never falls back to demo data. Only verified pharmacies are shown; an unknown or
unverified one is "Page not found". The route is still "planned": the gateway does
not mount it yet and its auth middleware treats `/api/v1/pharmacies/…` as
pharmacy-staff only, so today the page says "Pharmacy details are not available
right now" (a plain-text 404, 401, 403 or 501 all mean this). `GET
/api/v1/pharmacies/nearby` cannot stand in: it returns only IDs and distances.

- In gateway mode a failing or unreachable gateway is shown as an error. The app
  never replaces a failed search with demo stock.
- With `USE_DEMO_DATA=true` the app uses demo data from `lib/mock-data.ts` (built
  from `deploy/seed/`), and the results page says "Showing demo data".
- One exception in gateway mode: a hold carries only IDs and the gateway has no
  product or pharmacy name route yet, so the holds pages take names from the
  seed list in `lib/mock-data.ts` (`holdNames` in `lib/hold-format.ts`). For a
  seeded ID these are the same names as in the database; any other ID shows
  "Medicine" and "Pharmacy". No demo stock or demo holds are shown.

## Run

Requires Node.js 20+.

```bash
npm ci
npm run dev             # http://localhost:3000, live data from the gateway on :8080 (`make up`)
npm run dev:demo        # the same on demo data, no gateway needed
npm run dev -- -p 3071  # another port, if 3000 is taken
```

With the whole stack, `make up` at the repository root also runs this app in
Docker (see the root README). To try every error state with real network calls
but without the stack, run the stand-in gateway in another terminal:

```bash
npm run dev:gateway     # stand-in gateway on http://localhost:8080
npm run dev
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
jsdom, so `lib/contrast.test.ts` checks every colour pairing in `app/globals.css`
against the WCAG contrast ratios instead. When you add a colour token, add its
pairs to that test.

Mobile: the layout is one column below 40 rem. Buttons and fields are at least
2.75 rem (44 px) tall, text wraps instead of overflowing, and the page sets the
viewport meta tag. Check it by hand at 320 and 360 px wide.
