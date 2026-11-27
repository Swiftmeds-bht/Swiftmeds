# Load tests (k6)

Load tests for SRS-NFR-03 (test plan TC-035 and TC-036, ticket SCRUM-77).

| Script | What it does | Pass when |
|---|---|---|
| `search.js` | 100 virtual users search for seeded medicines around Thimphu, Paro and Chukha for 2 minutes, with 1 to 3 s think time | search p95 < 2 s, < 1% failed requests, > 99% checks pass |
| `reservation-burst.js` | 50 patients reserve the **same last unit** at the same moment, then wait for the saga to accept or reject each hold | no oversold stock (`oversold_units == 0`), every hold resolves, create-hold p95 < 2 s |

The thresholds are in the scripts, so `k6 run` exits non-zero when one fails (CI can gate on it).

## Running

Install [k6](https://grafana.com/docs/k6/latest/set-up/install-k6/), start the stack and run the
scripts **from this folder** (the summaries are written to `results/`):

```bash
make up
cd tests/load
k6 run search.js
k6 run reservation-burst.js
```

| Variable | Default | Meaning |
|---|---|---|
| `BASE_URL` | `http://localhost:8080` | Gateway URL |
| `VUS`, `HOLD` | `100`, `2m` | `search.js`: users and how long to hold them |
| `BURST_VUS` | `50` | `reservation-burst.js`: patients in the burst |
| `PATIENT_TOKENS` | registers new patients | comma-separated patient access tokens to reuse |
| `STAFF_TOKEN` | unset | staff token for pharmacy 01: resets the item to exactly 1 available unit first and also checks `quantityReserved <= quantityOnHand` afterwards |
| `CLEANUP` | `true` | cancel the winning hold afterwards so the test can be rerun |
| `PHARMACY_ID`, `PRODUCT_ID`, `PRODUCT_QUERY` | seed pharmacy 01 / product `…0002` / `Paracetamol Syrup` | the item to burst on |

The burst uses the seed row in `deploy/seed/stock.csv` where pharmacy 01 has exactly one pack of
product `10000000-0000-4000-8000-000000000002`. Without `STAFF_TOKEN`, a second run needs that unit
to be free again (the default cleanup cancels the winning hold).

### How "no oversell" is checked

After the burst, `teardown()` asks every patient in the burst for their `ACTIVE` holds and counts
the ones for the burst item. More `ACTIVE` holds than units available before the burst is oversold
stock. With `STAFF_TOKEN`, a reserved quantity above the quantity on hand also counts. Search must
never show a negative quantity.

## Dependencies

- `search.js` needs only the public search route (implemented).
- `reservation-burst.js` needs the gateway auth routes (SCRUM-115) and holds routes (SCRUM-54).

## Results

Each run writes `results/<script>.md` (and the full k6 JSON). Paste the Markdown block below and
into the test summary report.

| Date | Script | Environment | Result |
|---|---|---|---|
| _pending_ | `search.js` | Compose, 100 VUs | _run on the full stack_ |
| 18 Nov 2026 | `search.js` | Kubernetes prod overlay (laptop, k3d), 100 VUs for 2 min; both Traefik pods deleted during the run (SCRUM-135) | **pass**: 6,976 requests, 0 failed, p95 14 ms, max 121 ms (`results/search.md`) |
| _pending_ | `reservation-burst.js` | Compose, 50 patients | _blocked on SCRUM-115 and SCRUM-54_ |
