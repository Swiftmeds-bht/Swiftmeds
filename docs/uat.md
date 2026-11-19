# User Acceptance Testing

## 1. Purpose and scope

This document defines the demo-environment UAT script and future participant
results for the external SwiftMed SRS supplied for SCRUM-80. The formal
**US-01 to US-15** user-story IDs supersede the temporary test-plan IDs for
UAT purposes. Until the SRS is committed to the repository, [Section 7 of the test plan](test-plan.md)
remains the executable traceability baseline.

### Step 0 readiness check

| Story | Participant/UI/API surface | Today's verdict | UAT disposition |
|---|---|---|---|
| US-01 | Patient app search/results; search route implemented | PARTLY READY | Address is shown as available seed metadata; opening-hours UI and 95th-percentile timing need team checks |
| US-02 | Patient substitutions and alternative UI | NOT READY | No confirmed qualifying demo substitute pair; do not mock or bypass the missing data |
| US-03 | Patient price sorting | NOT READY | No price-sort control or sort behavior is available |
| US-04 | Patient reserve/holds UI and hold routes | READY | Scripted |
| US-05 | Holds and Expiry services | READY | Scripted as a team/session state check |
| US-06 | Gateway feedback/trust routes; patient feedback UI missing | PARTLY READY | Do not run participant feedback until a participant-facing form exists |
| US-07 | Watchlist app/routes; SMS/push/email and language delivery missing | PARTLY READY | Test subscription/restock alert only; do not claim multi-channel delivery |
| US-08 | Register/login UI and Auth gateway routes | PARTLY READY | Test registration/login/token rejection; saved location/language UI is not confirmed |
| US-09 | Pharmacy registration flow | NOT READY | No participant registration flow is confirmed |
| US-10 | Pharmacy CSV upload and stock-sync routes | READY | Scripted |
| US-11 | Dashboard quantity/price management | PARTLY READY | Test quantity and CSV price behavior; do not claim an unimplemented single-medicine price UI |
| US-12 | Pharmacy dashboard pickup-code collection | READY | Scripted |
| US-13 | Batch expiry, expiry warning and CSV expiry dates | NOT READY | No confirmed batch-expiry implementation |
| US-14 | Admin pharmacy approval | NOT READY | No confirmed admin approval flow |
| US-15 | Admin suspicious-price review | NOT READY | No confirmed admin suspicious-price flow |

## 2. Environment

- Demo start:
  `docker compose -f deploy/docker-compose.yml up -d`
  or `make up`.
- Gateway: `http://localhost:8080`
- SwiftMed (patients): `http://localhost:3000`
- Pharmacy portal: `http://localhost:3000/pharmacy-portal` (staff sign in at `/login`)
- RabbitMQ management UI: `http://localhost:15672`
- Traefik dashboard: `http://localhost:8081/dashboard/`

The commit SHA against which a session ran must be recorded by the facilitator:
`PENDING`.

Demo seed data comes from `deploy/seed/medicines.csv`,
`deploy/seed/pharmacies.csv`, `deploy/seed/prices.demo.csv`, and
`deploy/seed/stock.csv`. Pharmacy names are demo labels. Credentials are shared
outside git. No `.env`, token, real name, phone number, licence number, or
recording is stored here.

Reset procedure between sessions:

1. Stop active browser sessions and sign out both roles.
2. From the repository root, run `make down` to stop the Compose stack while
   keeping local data volumes.
3. Start `make up` and wait for the seed container.
4. Run `make seed` once more to reapply deterministic demo fixtures.
5. Confirm the demo patient and seeded pharmacists can sign in.
6. **If the session needs a fully clean database, `docker compose -f deploy/docker-compose.yml --env-file .env down -v` deletes all local Postgres and RabbitMQ volumes. This is destructive and must only be used with facilitator approval.**

Facilitators must also reset or collect any hold, stock update, watchlist
subscription, or feedback state created by a prior participant before the next
session. Existing active holds are not removed by repeated seeding.

## 3. Participants and sessions

The planning capacity is five patient participants and five pharmacist
participants. Replace no participant code with a real name.

| Session | Date | Role | Participant code | Facilitator | Observer | Language used | Consent noted |
|---|---|---|---|---|---|---|---|
| PENDING | PENDING | Patient | P1 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Patient | P2 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Patient | P3 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Patient | P4 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Patient | P5 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Pharmacist | Ph1 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Pharmacist | Ph2 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Pharmacist | Ph3 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Pharmacist | Ph4 | PENDING | PENDING | PENDING | PENDING |
| PENDING | PENDING | Pharmacist | Ph5 | PENDING | PENDING | PENDING | PENDING |

## 4. Session protocol

1. Start with the environment reset procedure and confirm the commit SHA.
2. Record verbal consent for participation and for note-taking, without
   recording names or personal details.
3. Give the participant only the task title and a time-box. Do not coach.
4. Ask the participant to think aloud and observe whether each task is
   completed unaided.
5. For every task record: completed unaided (`yes/no`), help steps, time taken,
   confusing wording, and a confidence rating from 1 to 5.
6. After the tasks, ask open questions about usefulness, preferred language,
   and whether the pickup-code flow is clear.
7. NFR checks are performed by the facilitator/team, not by patients or
   pharmacists.
8. Log only participant code, role, story ID, result, and Jira defect reference.
   Store no real personal data or secrets in this repository.

## 5. UAT script

### US-01 - Search for a medicine

- **Requirements / TCs:** FR-01 to FR-04, NFR-01, NFR-05, NFR-06 / TC-001, TC-002, TC-003
- **Role:** Patient
- **Precondition:** Patient is on `http://localhost:3000`; use demo location
  Thimphu.
- **Seed:** Search `Demo Paracetamol 500`, `500mg`, or generic `Paracetamol`.

1. Open the patient app and enter the medicine name.
2. Select Thimphu and search within the default 5000 m radius.
3. Repeat with the generic molecule and strength if asked.

**Expected:** Matching medicine and strength appear with verified nearby
pharmacies.

**Pass criteria:** The participant finds the medicine and can distinguish the
brand/generic and strength without assistance.

### US-03 - Sort results by price

- **Requirements / TCs:** FR-06
- **Role:** Patient
- **Precondition:** NOT TESTED: no price-sort control or sort behavior is confirmed
  in the patient app today.
- **Seed:** Do not invent a sort.

**Pass criteria:** Pending implementation; a real participant must be able to
sort low to high, with no-price pharmacies at the end.

### US-11 - Update stock and price

- **Requirements / TCs:** FR-13 / TC-026
- **Role:** Pharmacist
- **Precondition:** Sign in with a seeded verified pharmacy account.

1. Open the stock table.
2. Update the quantity for one demo medicine.
3. Where the demo CSV template supplies a price, update and review the price
   through the supported flow.

**Expected:** Quantity and price are updated; the update time is used for
freshness.

**Pass criteria:** The pharmacist can update quantity and price using only the
confirmed participant-facing flow.

### US-02 - See an alternative

- **Requirements / TCs:** FR-05 / TC-021, TC-022
- **Role:** Patient
- **Precondition:** NOT TESTED: no confirmed same-class, same-strength
  seeded alternative is available for a real participant session.
- **Seed:** Do not invent or mock an alternative.

**Expected:** When no stocked match exists, at least one safe alternative is
shown if available, and every alternative says why it was suggested.

**Pass criteria:** Pending implementation and seed confirmation. No medicine
from another therapeutic class may be suggested.

### US-04 - Reserve a medicine

- **Requirements / TCs:** FR-07, NFR-02 / TC-010, TC-012
- **Role:** Patient
- **Precondition:** Search and sign in as the demo patient.
- **Seed:** `Demo Ibuprofen 400`, Thimphu 02, 24 units.

1. Press Reserve for Demo Pharmacy Thimphu 02.
2. Reserve one unit and confirm.
3. Open My holds and wait until the hold becomes active.

**Expected:** The hold becomes active once and shows a pickup code and expiry
time.

**Pass criteria:** The participant receives the code and understands when it
must be used.

### US-05 - Release expired holds

- **Requirements / TCs:** FR-08, NFR-03 / TC-011, TC-015
- **Role:** Patient / facilitator
- **Precondition:** Create an active demo hold and allow or trigger its configured
  expiry/sweep.

1. Confirm the hold is active.
2. Trigger the expiry state once and observe the released stock.
3. Repeat the expiry action.

**Expected:** The hold becomes expired, stock is returned once, and repeating
the job changes nothing.

**Pass criteria:** The participant/team observes one stock return and an
idempotent repeated expiry.

### US-12 - Confirm a pickup code

- **Requirements / TCs:** FR-14 / TC-014
- **Role:** Pharmacist
- **Precondition:** A different session created an active hold for Thimphu 02.
  Sign in at `/login` with the seeded pharmacist for that pharmacy; SwiftMed opens the pharmacy portal.

1. Find the active hold.
2. Enter an obviously wrong code once and observe the response.
3. Enter the patient's valid code.
4. Try to collect the same hold again.

**Expected:** Wrong code is refused; valid code collects once; reused code is
refused.

**Pass criteria:** The pharmacist understands whether each attempt succeeded
and the hold cannot be collected twice.

### US-07 - Follow an unavailable medicine

- **Requirements / TCs:** FR-22, FR-23 / TC-023, TC-024
- **Role:** Patient
- **Precondition:** Patient is signed in and has selected a Thimphu location.
  The facilitator records a start state with no eligible stock.

1. Search for the selected unavailable seeded medicine.
2. Press **Notify me when it is back**.
3. Open Watchlist and confirm the subscription appears.
4. Follow the team-controlled stock-restock step, if one is run.

**Expected:** The subscription appears once and an eligible restock produces one
alert.

**Pass criteria:** The participant understands waiting/alert state and receives
no duplicate alert.

### US-10 - Upload stock

- **Requirements / TCs:** FR-10 to FR-12 / TC-026
- **Role:** Pharmacist
- **Precondition:** Sign in with a seeded verified pharmacy account.

1. Download or prepare the demo stock CSV template.
2. Include valid rows and at least one intentionally rejected row.
3. Upload the file and open the import report.

**Expected:** Valid rows apply, own product codes match the catalogue, and
failed rows are listed with reasons.

**Pass criteria:** The pharmacist can distinguish applied and rejected rows
without help.

### US-08 - Register and log in

- **Requirements / TCs:** FR-24, FR-25, NFR-04 / TC-027, TC-028
- **Role:** Patient, pharmacy staff or admin
- **Precondition:** NOT READY for complete role coverage until role-specific
  participant flows are confirmed.

**Pass criteria:** Pending real sessions: registration/log-in works for the
confirmed role, wrong passwords fail, and requests without valid tokens fail.

### US-09 - Register a pharmacy

- **Requirements / TCs:** FR-09 / TC-025
- **Role:** Pharmacy owner
- **Precondition:** NOT READY: no confirmed registration with licence number,
  address and opening hours is available in the demo UI.

**Pass criteria:** Pending implementation and a real participant session.

### US-13 - Track expiry dates

- **Requirements / TCs:** FR-17 to FR-19
- **Role:** Pharmacist
- **Precondition:** NOT READY: batch expiry dates and expiry warnings are not
  confirmed as participant-facing demo features.

**Pass criteria:** Pending implementation and real participant use.

### US-14 - Approve a pharmacy

- **Requirements / TCs:** FR-15 / TC-025
- **Role:** Admin
- **Precondition:** NOT READY: no confirmed admin approval flow.

**Pass criteria:** Pending implementation and real admin session.

### US-15 - Check suspicious prices

- **Requirements / TCs:** FR-16
- **Role:** Admin
- **Precondition:** NOT READY: no confirmed suspicious-price flagging or review
  flow.

**Pass criteria:** Pending implementation and real admin session.

### US-06 - Give feedback

- **Requirements / TCs:** FR-20, FR-21 / TC-033
- **Role:** Patient and pharmacist
- **Precondition:** The participant has a collected hold.
- **Current readiness:** PARTLY READY. The gateway feedback route is implemented,
  but the patient app has no feedback form.

Do not run this as a participant task until the missing UI is available. When
available, the team will provide a plain-language task to confirm in-stock
accuracy and rating; it must not call the API directly as a substitute for the
participant UI.

**Pass criteria:** Pending the UI and the real participant session.

### NFR-02 - Actionable errors and request IDs

- **Requirements / TCs:** NFR-02 / TC-034
- **Role:** Facilitator
- **Precondition:** One intentional demo failure is triggered without changing
  production data.

1. Trigger a team-approved failure or validation error.
2. Copy the response error and `X-Request-ID`.
3. Confirm logs are searchable for the same request ID.

**Expected:** The participant sees an actionable error, and the team can trace
the request by ID.

**Pass criteria:** Team completes the trace without exposing secrets.

### NFR-06 - Demo-only and non-production testing

- **Requirements / TCs:** NFR-06, NFR-07 / TC-034, TC-040
- **Role:** Facilitator

1. Confirm the environment uses `make up`/Docker Compose demo seed data.
2. Confirm credentials and tokens are not committed or pasted into this file.
3. Confirm participant codes are used instead of names or personal data.

**Expected:** No real patient data or production credential is used.

**Pass criteria:** The team records only demo evidence and codes.

## 6. Results

Use only `PASS`, `PASS WITH COMMENTS`, `FAIL`, `NOT TESTED`, or `PENDING`.

| Story | US/requirements | Participant | Result | Notes | Defect |
|---|---|---|---|---|---|
| Search | US-01 / FR-01-FR-04 | PENDING | PENDING | PENDING | PENDING |
| Alternative | US-02 / FR-05 | PENDING | NOT TESTED | `NOT READY`: no confirmed demo alternative | PENDING |
| Sort by price | US-03 / FR-06 | PENDING | NOT TESTED | `NOT READY`: no sort control | PENDING |
| Reserve | US-04 / FR-07 | PENDING | PENDING | PENDING | PENDING |
| Release expired holds | US-05 / FR-08 | Team/Pending participant | PENDING | PENDING | PENDING |
| Feedback | US-06 / FR-20-FR-21 | PENDING | NOT TESTED | `PARTLY READY`: participant UI missing | PENDING |
| Follow unavailable medicine | US-07 / FR-22-FR-23 | PENDING | PENDING | PENDING | PENDING |
| Register and log in | US-08 / FR-24-FR-25 | PENDING | PENDING | PENDING | PENDING |
| Register pharmacy | US-09 / FR-09 | PENDING | NOT TESTED | `NOT READY`: registration flow missing | PENDING |
| Upload stock | US-10 / FR-10-FR-12 | PENDING | PENDING | PENDING | PENDING |
| Update stock and price | US-11 / FR-13 | PENDING | PENDING | PENDING | PENDING |
| Confirm pickup code | US-12 / FR-14 | PENDING | PENDING | PENDING | PENDING |
| Track expiry dates | US-13 / FR-17-FR-19 | PENDING | NOT TESTED | `NOT READY`: batch expiry flow missing | PENDING |
| Approve pharmacy | US-14 / FR-15 | PENDING | NOT TESTED | `NOT READY`: admin approval flow missing | PENDING |
| Suspicious prices | US-15 / FR-16 | PENDING | NOT TESTED | `NOT READY`: admin review flow missing | PENDING |
| Observable errors | NFR-02 | Team | PENDING | PENDING | PENDING |
| Demo data hygiene | NFR-06 | Team | PENDING | PENDING | PENDING |

## 7. Defect log

| Defect | Jira key | Story | Severity | Summary | Fix branch | Status |
|---|---|---|---|---|---|---|

Severity scale:

- **Critical:** wrong stock, wrong reservation or pickup outcome, security or
  data problem, or a P0 story cannot be completed.
- **Major:** the story completes only with help or an inconvenient workaround.
- **Minor:** wording, layout or small friction.
- **Cosmetic:** appearance-only issue with no workflow impact.

### Ready-to-paste Jira Bug template

```text
Title:
Story and US/requirements ID:
Environment:
Commit SHA:
Participant code:
Severity:

Steps to reproduce:
1.
2.
3.

Expected:
Actual:

Request ID (if any):
Jira suggested fix branch: fix/SCRUM-<n>-short-name
```

## 8. Sign-off

The Product Owner is the sign-off authority. Leave the signing name and date
blank until real approval is recorded.

| Story | US/requirements | Outcome | Defect key(s) | Signed off by | Date |
|---|---|---|---|---|---|
| Search | US-01 / FR-01-FR-04 | PENDING | PENDING | PENDING | PENDING |
| Alternative | US-02 / FR-05 | NOT TESTED | PENDING | PENDING | PENDING |
| Sort by price | US-03 / FR-06 | NOT TESTED | PENDING | PENDING | PENDING |
| Reserve | US-04 / FR-07 | PENDING | PENDING | PENDING | PENDING |
| Release expired holds | US-05 / FR-08 | PENDING | PENDING | PENDING | PENDING |
| Feedback | US-06 / FR-20-FR-21 | NOT TESTED | PENDING | PENDING | PENDING |
| Follow unavailable medicine | US-07 / FR-22-FR-23 | PENDING | PENDING | PENDING | PENDING |
| Register and log in | US-08 / FR-24-FR-25 | PENDING | PENDING | PENDING | PENDING |
| Register pharmacy | US-09 / FR-09 | NOT TESTED | PENDING | PENDING | PENDING |
| Upload stock | US-10 / FR-10-FR-12 | PENDING | PENDING | PENDING | PENDING |
| Update stock and price | US-11 / FR-13 | PENDING | PENDING | PENDING | PENDING |
| Confirm pickup code | US-12 / FR-14 | PENDING | PENDING | PENDING | PENDING |
| Track expiry dates | US-13 / FR-17-FR-19 | NOT TESTED | PENDING | PENDING | PENDING |
| Approve pharmacy | US-14 / FR-15 | NOT TESTED | PENDING | PENDING | PENDING |
| Suspicious prices | US-15 / FR-16 | NOT TESTED | PENDING | PENDING | PENDING |
| Observable errors | NFR-02 | PENDING | PENDING | PENDING | PENDING |
| Demo data hygiene | NFR-06-NFR-07 | PENDING | PENDING | PENDING | PENDING |

A story is signed off only after a real participant run passes and no open
Critical or Major defect remains. A recorded Product Owner waiver is required
before an unresolved exception can affect sign-off.

## 9. Open issues and waivers

| Item | Decision | Decided by | Date |
|---|---|---|---|

## 10. Traceability

These rows are not covered until the UAT result is `PASS` or
`PASS WITH COMMENTS`.

| US/requirement | Story | TC | UAT result | Defect |
|---|---|---|---|---|
| FR-01-FR-04 / US-01 | Search for a medicine | TC-001, TC-002, TC-003 | PENDING | PENDING |
| FR-05 / US-02 | See an alternative | TC-021, TC-022 | NOT TESTED | PENDING |
| FR-06 / US-03 | Sort by price | PENDING | NOT TESTED | PENDING |
| FR-07 / US-04 | Reserve a medicine | TC-010, TC-012 | PENDING | PENDING |
| FR-08 / US-05 | Release expired holds | TC-011, TC-015 | PENDING | PENDING |
| FR-09 / US-09 | Register a pharmacy | TC-025 | NOT TESTED | PENDING |
| FR-10-FR-12 / US-10 | Upload stock | TC-026 | PENDING | PENDING |
| FR-13 / US-11 | Update stock and price | TC-026 | PENDING | PENDING |
| FR-14 / US-12 | Confirm pickup code | TC-014 | PENDING | PENDING |
| FR-15 / US-14 | Approve a pharmacy | TC-025 | NOT TESTED | PENDING |
| FR-16 / US-15 | Check suspicious prices | PENDING | NOT TESTED | PENDING |
| FR-17-FR-19 / US-13 | Track expiry dates | PENDING | NOT TESTED | PENDING |
| FR-20-FR-21 / US-06 | Give feedback | TC-033 | NOT TESTED | PENDING |
| FR-22-FR-23 / US-07 | Follow unavailable medicine | TC-023, TC-024 | PENDING | PENDING |
| FR-24-FR-25 / US-08 | Register and log in | TC-027, TC-028 | PENDING | PENDING |
| NFR-01, NFR-05, NFR-06 / US-01 | Performance, mobile, degraded search | TC-035, TC-037, TC-039 | PENDING | PENDING |
| NFR-02 / US-04 | No negative stock under concurrency | TC-012 | PENDING | PENDING |
| NFR-03 / US-05 | Idempotent expiry | TC-015 | PENDING | PENDING |
| NFR-04 / US-08 | Password and token security | TC-027, TC-028 | PENDING | PENDING |
| NFR-07 | All requirements have tests | TC register | PENDING | PENDING |
