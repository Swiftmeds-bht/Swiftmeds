# Patient web app

Next.js app where patients search for a medicine and see which nearby pharmacies
have it in stock. Owner: Pema. Responsive: works on phones and desktops.

## Pages

| Route | What it shows |
|---|---|
| `/` | Search form: medicine name (with suggestions), location, radius |
| `/results?q=…&place=…&radius=…` | Matching medicines with in-stock pharmacies (best match first and highlighted: distance, price, stock, and freshness as 1 to 3 bars with words), and same-molecule alternatives when nothing is in stock. The search sits in one compact bar; the town and radius fold into a "Near …, within … · Change" line (a native `<details>`, so it works without JavaScript) |
| `/results?q=…&lat=…&lng=…&radius=…` | The same, searched from the patient's device location |
| `/pharmacy/[id]` | Pharmacy detail (placeholder: address, dzongkhag, map link and a link back to search; opening hours and stock list come later) |
| `/reserve?product=…&pharmacy=…` | Confirm a reservation: medicine, pharmacy, price, how many units (1 to 5, no more than in stock), and the total to pay at the counter, which updates with the number of units. Opened from Reserve on a result |
| `/holds/[id]` | One hold: "Checking stock" (refreshes every 2 s), then the pickup code on a ticket with a countdown, or why it was not held, expired or cancelled. When a hold has just turned active (status changed in the last 15 s), the code's digits rise into place once; reloads do not replay it, and reduced motion turns it off |
| `/holds` | My holds: current (with Cancel) and past |
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
| Some details missing | A note when the gateway reports optional services as down (`x-search-degraded` header) |

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

**Session.** Signing in stores the access token in an httpOnly, SameSite=Lax cookie
(`lib/session.ts`, `lib/auth.ts`), so page scripts cannot read it. All gateway
calls with the token happen on the server (`app/actions.ts`, server components).
When the gateway rejects the token, the page sends the patient to sign in again
(`/session-ended` clears the cookie) and back afterwards. The `next` address after
signing in must be a path on this site, so a crafted link cannot send a patient
elsewhere. Refresh tokens are not used yet: the patient signs in again when the
access token expires (`JWT_TTL`, 15 minutes by default).

**Gateway routes** (`lib/holds.ts`): `POST /api/v1/auth/register`, `POST /api/v1/auth/login`,
`POST /api/v1/holds`, `GET /api/v1/holds/{id}`, `GET /api/v1/holds`,
`POST /api/v1/holds/{id}/cancel`. Most are still "planned" in the API spec; until
they exist, a gateway answering 404 shows "This is not available yet".

**Demo mode** (no `GATEWAY_URL`, `lib/demo-holds.ts`): accounts and holds live in
the server's memory and are lost on restart. Sign in as `patient@demo.bt` /
`swiftmed-demo`, or create an account. A hold is pending for 2 seconds, then active
for 30 minutes. It is rejected when more units are asked for than are left, or when
the pharmacy had only its last unit (the demo pretends someone else got it first,
so the rejected screen can be seen: reserve "Demo Paracetamol Syrup" at Thimphu 01).

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
jsdom, so `lib/contrast.test.ts` checks every colour pairing in `app/globals.css`
against the WCAG contrast ratios instead. When you add a colour token, add its
pairs to that test.

Mobile: the layout is one column below 40 rem. Buttons and fields are at least
2.75 rem (44 px) tall, text wraps instead of overflowing, and the page sets the
viewport meta tag. Check it by hand at 320 and 360 px wide.
