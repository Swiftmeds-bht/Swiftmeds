# SwiftMed Pharmacy Dashboard

A Next.js dashboard for pharmacy staff (`ROLE_PHARMACY_STAFF`):

- **Stock** (SCRUM-58): see on-hand, reserved and available quantities and set the
  quantity on hand. Reserved units are protected: the gateway refuses a quantity below
  the reserved amount.
- **Holds** (SCRUM-58): see the pharmacy's active holds and collect one with the pickup
  code the patient shows. A wrong code is refused and the hold stays active.
- **CSV upload** (SCRUM-70): upload a stock CSV to the Stock Sync service and see the
  per-row import report. The template at `/upload/template` is pre-filled with the
  pharmacy's current stock.

All calls go through the API Gateway (see `docs/api/openapi.yaml`); the dashboard never
talks to a service directly. The access token is kept in an HTTP-only cookie and the
pharmacy ID is read from its `pharmacy_id` claim.

## Requirements

- Node.js 20 or newer
- The SwiftMed API Gateway, or the dev stand-in below

## Local setup

```bash
npm install
npm run dev:gateway                             # stand-in gateway on http://localhost:8080
GATEWAY_URL=http://localhost:8080 npm run dev   # http://localhost:3000
```

Or create `web/pharmacy-dashboard/.env.local` with:

```env
GATEWAY_URL=http://localhost:8080
```

### Dev gateway

`scripts/dev-gateway.mjs` answers the routes the dashboard uses, with in-memory data,
so the dashboard can run without the rest of the stack. The real gateway now serves
all of them: auth (SCRUM-115), holds (SCRUM-54), stock (SCRUM-58) and stock-sync
(SCRUM-70). To use it instead, run the Compose stack and point `GATEWAY_URL` at it.

| What | Value |
|---|---|
| Staff login | `staff@demo.bt` / `password` |
| Patient login (refused by the dashboard) | `patient@demo.bt` / `password` |
| Pickup code of every demo hold | `482913` |

## CSV format

```csv
product_id,quantity,price_chhertum
3f6c7b2e-1d44-4d6f-9b0a-0a4d5f2f8c10,40,4500
```

- `quantity` is the total on hand: a whole number, 0 or more.
- `price_chhertum` is optional; leave it blank to keep the current price.
- UTF-8, at most 1 MB. Row numbers in the report count the header as row 1.

## Checks

```bash
npm test          # Vitest unit tests
npm run typecheck
npm run lint
npm run build
```
