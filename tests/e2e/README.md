# End-to-end tests (Cypress)

Browser tests of the main journey, search -> reserve -> collect, against the
Docker Compose stack (SCRUM-72). They are written from the Gherkin scenarios in
[tests/features](../features) and use the real SwiftMed web app (patient pages and the pharmacy portal),
gateway and services, with no mocks.

| Spec | Scenarios | Test cases |
|---|---|---|
| [reserve_and_collect.cy.js](cypress/e2e/reserve_and_collect.cy.js) | Reserving gives a pickup code and reserves the stock; the pharmacist collects with the right code; a wrong code is refused; cancelling gives the stock back | TC-010, TC-014, TC-016 |
| [rejected_and_expired.cy.js](cypress/e2e/rejected_and_expired.cy.js) | Rejected when the stock ran out after the search; expired when not collected in time, with the stock released and the hold gone from the pharmacist's list | TC-031, TC-011, TC-015 |

## Run

Only Docker is needed: Cypress runs in its official image (`cypress/included`),
on the stack's network.

```bash
cp .env.example .env              # once
tests/e2e/run.sh --start-stack    # build, start and seed the stack with 45 s holds, then run the specs
tests/e2e/run.sh                  # run the specs against a stack that is already up
make e2e                          # the same as tests/e2e/run.sh --start-stack
```

Extra arguments go to `cypress run`, for example
`tests/e2e/run.sh --spec cypress/e2e/reserve_and_collect.cy.js`. Screenshots of
failures are saved in `tests/e2e/cypress/screenshots/`.

### Why the expiry test needs short holds

Holds refuses to expire a hold before its own `expires_at`, whatever time the
sweep is given, so the test must really wait for a hold to run out.
`--start-stack` starts Holds and Expiry with `E2E_HOLD_SECONDS` (default 45)
second holds. Against a stack started with `make up` (2-hour holds) the expiry
test is skipped and the others run. The test runs the sweep itself through the
gateway (`POST /api/v1/internal/expiry/sweep` with `WORKER_SHARED_SECRET`), as
the Cloudflare Worker would.

### Why Cypress talks to localhost

The web apps set their session cookies with the `Secure` flag in production, and
browsers accept those over plain HTTP only from `localhost`. Inside the Cypress
container, [scripts/forward.mjs](scripts/forward.mjs) forwards `localhost:3000`
(the web app, pharmacy portal included) and `localhost:8080` (gateway) to
Traefik, so the browser sees the same addresses as on a developer machine.

## Test data

- The seeded **Norzin Pharmacy** and **Paracetamol Tablets**
  (`deploy/seed`), and the pharmacist linked to that pharmacy,
  `demo-tim-001@demo.swiftmeds.test` (seeded by `make seed`).
- Every test signs up a new patient (`e2e-<time>@e2e.swiftmeds.test`), so tests
  never share holds or hit the 3-open-holds limit.
- Before a test the stock is set through the gateway, as the pharmacist, so that
  exactly the units the scenario needs are for sale. Holds left active by an
  earlier run keep their units: the on-hand quantity is set to reserved + wanted.

No real patient data is used (SRS-NFR-06).

## Run Cypress without Docker

```bash
cd tests/e2e
npm ci                                  # downloads the Cypress binary
E2E_PATIENT_URL=http://localhost:3000 npx cypress run
```

On Linux this needs Cypress's system libraries (see the Cypress docs). Settings:

| Variable | Default | Meaning |
|---|---|---|
| `E2E_PATIENT_URL` | `http://localhost:3000` | the SwiftMed web app (the pharmacy portal is at `/pharmacy-portal`) |
| `E2E_GATEWAY_URL` | `http://localhost:8080` | API gateway, for test setup |
| `E2E_HOLD_SECONDS` | `7200` | how long holds last in the stack; the expiry test runs only at 120 or less |
| `E2E_WORKER_SECRET` | empty | `WORKER_SHARED_SECRET`, to run the expiry sweep |
| `E2E_DEMO_PASSWORD` | `swiftmeds-demo` | password of the seeded pharmacist |
