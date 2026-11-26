# SwiftMed Test Plan

## 1. Purpose and scope

This plan defines how SwiftMed is verified from the service boundary through the
patient application and pharmacy dashboard. It covers the 20 Go microservices,
the REST gateway, gRPC calls, RabbitMQ events, scheduled Cloudflare Workers,
Postgres persistence, Next.js applications, and the Cypress and k6 test
fixtures described in the repository.

The plan is risk-based: tests for medication availability, reservation
correctness, expiry, identity, and stale data run before lower-risk reporting
and presentation checks. A test case is not considered passing merely because
an HTTP response has the expected shape; state, persisted data, emitted events,
authorization, and user-visible outcomes must also be verified where relevant.

### 1.1 Current requirements baseline

The repository currently contains no separate SRS file. Until one is added,
the requirement IDs in Section 7 are the traceability baseline derived from the
product and architecture statements in [README.md](../README.md). When the
formal SRS is committed, its IDs supersede these IDs and the matrix must be
updated before release. No requirement may be marked covered by inference:
each formal SRS requirement must have at least one matrix row and one executable
or reviewable test case.

### 1.2 Out of scope

Excluded by the project proposal (Section 2A), so not tested:

- Online payment, checkout, and home delivery; SwiftMed shows availability only.
- Uploading or storing prescriptions; pharmacists check prescriptions at the counter.
- Controlled and restricted medicines. The only test here is that they never
  appear in search or substitution results (TC-021).
- Clinical advice, diagnosis, or dispensing decisions.
- Automatic integration with pharmacy point-of-sale systems.
- Native Android or iOS applications.
- Benchmarking infrastructure unrelated to SwiftMed's gateway, services,
  databases, queues, and web applications.

## 2. Test levels and test types

| Level/type | Purpose and scope | Primary owner | Exit evidence |
|---|---|---|---|
| Unit | Domain rules, validation, ranking, expiry, freshness, adapters' error mapping, and worker handlers in isolation. Dependencies are replaced with fakes. | Service owner | `go test` results and coverage |
| Integration | A service with its Postgres schema, RabbitMQ exchange/queue, gRPC clients, and gateway contracts. Verify transactions, retries, idempotency, timeouts, and event payloads. | Service owner and QA | Repeatable environment plus logs and persisted-state assertions |
| End-to-end | Critical patient and pharmacy journeys through the gateway and Next.js applications: search, reserve, collect/expire, stock update, substitution, and watchlist notification. | QA | Cypress report, screenshots/video for failures |
| Load/performance | Gateway and critical service paths under expected and peak concurrent search, stock update, and reservation traffic. Include queue lag and database saturation. | QA and platform | k6 thresholds, resource and error-rate report |
| Accessibility | Keyboard-only, focus order, labels, names/roles, contrast, status announcements, and responsive behavior for patient and pharmacy journeys. | QA and frontend owners | axe report plus manual checklist |
| UAT | Bhutan patient and pharmacy workflows using realistic seeded medicines, locations, stock, and prices. Confirm usefulness, language, pickup-code flow, and operational acceptance. | Product owner and representative users | Signed scenario checklist and open-issue disposition |
| Regression | Re-run all critical-path tests and impacted service suites after a change, including contract and migration compatibility. | QA | CI artifact linked to the change |

## 3. Test strategy and techniques

### 3.1 Test design

- **Equivalence partitioning:** valid/invalid medicine names, molecule and
  strength formats, coordinates, radius, stock quantities, user roles, token
  states, pharmacy status, and notification channels.
- **Boundary-value analysis:** use the limits fixed in the `.proto` contracts:
  search radius 499/500/50000/50001 m (0 means the 5000 m default), hold
  quantity 0/1/5/6, stock `0/1/max`, password length 7/8/72/73 bytes, gateway
  port 0/1/65535/65536, hold duration just before/at/just after expiry,
  freshness and trust scores at 0.0 and 1.0, empty and maximum result sets, and
  k6 rate thresholds.
- **Decision tables:** reservation eligibility (verified pharmacy, available
  stock, active user, duplicate hold, and concurrent requests), search ranking
  tie-breakers, substitution matching, and notification eligibility.
- **State-transition testing:** the hold lifecycle in `holds.proto`:
  PENDING→ACTIVE (stock reserved), PENDING→REJECTED (stock unavailable),
  ACTIVE→COLLECTED/CANCELLED/EXPIRED; terminal states accept no further
  transition. Refresh tokens: issued→rotated or revoked (Logout), never reusable.
  Watchlist subscriptions: subscribed→notified (`notified_at` set, fires once)
  or unsubscribed. Freshness is not a state machine; it is a score that decays
  from 1.0 with age and resets on confirmation, so test it with boundary
  values instead.
- **Pairwise combinations:** role, stock state, pharmacy verification,
  freshness, network outcome, and notification channel where exhaustive
  combinations are disproportionate.
- **Contract testing:** generated protobuf and event schemas, REST status/error
  shapes, backward-compatible field evolution, and correlation/request IDs.
- **Property-based testing:** ranking is deterministic and paid placement never
  changes ordering; inventory never becomes negative; hold transitions are
  legal and idempotent.

### 3.2 Development practices

- **TDD:** write a failing unit test before implementing new domain behavior.
- **BDD:** keep critical user journeys in `tests/features` as Given/When/Then
  scenarios; link each scenario to a matrix test case.
- **Mutation testing:** run Gremlins against core business packages (Inventory,
  Holds, Ranking, Freshness, and Substitution). A surviving mutant in a
  safety-critical rule is a release blocker until the test suite is improved or
  the mutant is justified.
- **Failure injection:** test RabbitMQ redelivery, duplicate events, gRPC
  timeout/unavailable responses, database rollback, worker retry, and partial
  saga completion.

## 4. Tools and environments

| Concern | Tool/process |
|---|---|
| Go unit tests | `go test ./...` per Go module; repository `make test` (CI runs every module with `GOWORK=off` and `-race`) |
| Go integration tests | Build tag `integration` plus a live Postgres or RabbitMQ, one URL variable per suite (e.g. `HOLDS_TEST_DATABASE_URL`, `RABBITMQ_URL`). Without its variable a test is skipped. `scripts/integration-tests.sh` runs them all with `-race -v`; CI's **Go integration tests** job starts Postgres and RabbitMQ, sets every variable and fails if any test skips (SCRUM-116) |
| Saga integration tests | `tests/integration` (SCRUM-56): Postgres and RabbitMQ in testcontainers, the real Holds and Inventory; CI job **Saga integration tests** |
| Web app and Workers | CI jobs **Web app** (patient app: lint, typecheck, Vitest, build) and **Workers** (typecheck and tests for each Cloudflare Worker) |
| Coverage gate | `make coverage`: domain and app packages of every Go service must reach 65% (CI runs the same script) |
| Static quality gate | `make lint` and the CI configuration |
| Service dependencies | Docker Compose with Postgres, RabbitMQ, and Traefik |
| Browser E2E and fixtures | Cypress; scenarios under `tests/e2e` and `tests/features` |
| Load and soak | k6 scripts under `tests/load`; collect latency, errors, throughput, queue lag, and resource use |
| Accessibility | axe in Cypress plus manual keyboard and screen-reader checks |
| Mutation testing | Gremlins v0.5.0 on the core domain and app packages: `make mutation` (`scripts/mutation-tests.sh`) runs Ranking, Inventory and Holds and fails if a package is below the thresholds in its `.gremlins.yaml`. CI runs nightly and on pull requests labeled `run-mutation` (SCRUM-113) |
| Observability | Structured service logs, correlation IDs, RabbitMQ management data, and database assertions |

All automated tests must use deterministic seed data and isolated databases or
namespaces. Clock-dependent tests use an injectable clock or a controlled test
time. Secrets come from the test environment; `.env` files and real patient
data are never committed.

## 5. Entry, suspension, and exit criteria

### 5.1 Entry criteria

- The change has an approved requirement or defect reference and identified
  impacted services.
- Code builds with the supported Go and Node versions and generated protobuf
  code is current.
- Test data, migrations, service configuration, and Docker dependencies are
  available.
- Unit tests exist for new domain rules; test environments have no known
  blocking outage.

### 5.2 Suspension and resumption

Suspend a level when its environment is unavailable, test data is corrupt, or
an infrastructure defect prevents meaningful assertions. Record the blocker,
affected cases, and last trustworthy result. Resume from the failed or skipped
case after the environment is repaired; do not convert an infrastructure
failure into a pass.

### 5.3 Exit criteria

- All critical and high-risk cases pass; every failed or skipped case has an
  approved defect, waiver, or explicit scope decision.
- `make test` and `make lint` pass.
- Core services (Inventory, Holds, Ranking, Freshness, and Substitution) have
  at least 80% statement coverage; changed packages meet the repository's CI
  coverage gate.
- All SRS rows in the traceability matrix have a test case, an owner, and a
  recorded result.
- Critical Cypress journeys, accessibility checks, and agreed k6 thresholds
  pass.
- No unresolved release-blocking security, data-integrity, reservation-saga,
  or authorization defect remains.
- UAT scenarios are accepted by the product owner.

## 6. Coverage targets and quality gates

| Area | Target/gate |
|---|---|
| Core service statements | At least 80% for Inventory, Holds, Ranking, Freshness, and Substitution |
| New or changed domain branches | 100% of business-rule branches, including invalid and failure paths |
| Public REST/gRPC/event contracts | 100% of documented operations and error classes exercised |
| Critical state transitions | 100% of legal transitions and representative illegal transitions |
| Critical user journeys | 100% pass in Chromium and the supported responsive viewport set |
| Accessibility | No critical or serious axe violations; all critical journeys keyboard-completable |
| Load | Search: p95 below 2 s at 100 concurrent users (proposal objective 4), error rate below 1%. Reservation: no oversell and p95 below 2 s at the same load |
| Mutation | No surviving mutant in a core safety or reservation rule without a documented waiver |
| Traceability | 100% of formal SRS requirements mapped to at least one test case |

Coverage is evidence, not a substitute for assertion quality. A high line
percentage with no state, event, or authorization assertion does not satisfy a
gate.

## 7. Traceability matrix

The following baseline requirements are intentionally testable statements.
`UT` denotes unit tests, `IT` integration tests, `E2E` Cypress tests, `LT` k6
tests, `AT` accessibility tests, and `UAT` acceptance tests.

| Requirement | Requirement statement | Test cases | Level/type | Priority |
|---|---|---|---|---|
| SRS-F-01 | A patient can search for a medicine by product, molecule, and strength. | TC-001, TC-002 | IT, E2E, UAT | P0 |
| SRS-F-02 | Search results include verified nearby pharmacies with currently available stock. | TC-003, TC-004 | IT, E2E | P0 |
| SRS-F-03 | Search accepts location and radius and handles empty, invalid, and boundary values safely. | TC-005, TC-006 | UT, IT | P0 |
| SRS-F-04 | Results are ranked only by distance, stock-confirmation freshness, pharmacy accuracy record, and price; paid placement cannot affect ranking. | TC-007, TC-008 | UT, IT, Mutation | P0 |
| SRS-F-05 | A patient can view pharmacy, stock, freshness, distance, and price details for a result. | TC-009 | E2E, UAT, AT | P1 |
| SRS-F-06 | A patient can reserve available stock for a short hold and receives a pickup code. | TC-010, TC-011 | IT, E2E, UAT | P0 |
| SRS-F-07 | Concurrent or duplicate reservations cannot oversell inventory, and reservation processing is idempotent. | TC-012, TC-013 | UT, IT, LT | P0 |
| SRS-F-08 | A reservation can be collected with a valid pickup code and cannot be collected twice or with an invalid code. | TC-014 | IT, E2E, UAT | P0 |
| SRS-F-09 | Expired or cancelled reservations release stock and cannot be collected. | TC-015, TC-016 | UT, IT, E2E | P0 |
| SRS-F-10 | Stock updates are persisted, synchronized, and reflected in search results. | TC-017, TC-018 | IT, E2E | P0 |
| SRS-F-11 | Stock freshness decays on schedule and stale stock is clearly excluded or labelled according to the product rule. | TC-019, TC-020 | UT, IT, E2E | P0 |
| SRS-F-12 | When requested stock is unavailable, the system suggests products with the same molecule and strength. | TC-021, TC-022 | UT, E2E, UAT | P1 |
| SRS-F-13 | A patient can watch a medicine and is notified when matching stock returns. | TC-023, TC-024 | IT, E2E, UAT | P1 |
| SRS-F-14 | Only registered and verified pharmacies contribute eligible listings, and pharmacy users can manage their stock through the dashboard. | TC-025, TC-026, TC-041, TC-042 | IT, E2E, UAT | P0 |
| SRS-F-15 | Authentication, token expiry, roles, and authorization prevent patients and pharmacy users from accessing each other's protected operations. | TC-027, TC-028 | UT, IT, E2E | P0 |
| SRS-F-16 | Gateway REST requests are validated and translated to the correct gRPC operation and documented error response. | TC-029, TC-030 | IT, Contract | P0 |
| SRS-F-17 | Services exchange reservation and stock events through RabbitMQ with correlation, retry, duplicate, and failure handling. | TC-031, TC-032 | IT, Failure injection | P0 |
| SRS-F-18 | Analytics and feedback record the agreed pharmacy accuracy and user feedback data without changing the ranking inputs outside their defined rules. | TC-033 | IT, UAT | P1 |
| SRS-NFR-01 | Core reservation and inventory operations preserve atomicity, consistency, and non-negative stock across retries and failures. | TC-012, TC-013, TC-031 | UT, IT, Mutation | P0 |
| SRS-NFR-02 | The system remains observable through structured logs, correlation IDs, event outcomes, and actionable errors. | TC-034 | IT, UAT | P0 |
| SRS-NFR-03 | Search returns within 2 s for 95% of requests at 100 concurrent users, and reservations stay correct at the same load. | TC-035, TC-036 | LT | P0 |
| SRS-NFR-04 | Patient and pharmacy web journeys meet WCAG-oriented keyboard, focus, naming, contrast, and status-announcement checks. | TC-037, TC-038 | AT, E2E | P0 |
| SRS-NFR-05 | Public contracts, migrations, and generated protobuf code remain backward compatible for supported clients and services. | TC-039 | Contract, IT, Regression | P1 |
| SRS-NFR-06 | Test environments isolate data, use non-production credentials, and never expose secrets or real patient data. | TC-040 | IT, UAT, Review | P0 |

### 7.1 Test case register

| ID | Short description | Main assertions |
|---|---|---|
| TC-001 | Valid product search | Matching product and strength are returned |
| TC-002 | Molecule/strength equivalence | Equivalent molecule search returns intended catalog entries |
| TC-003 | Verified pharmacy availability | Pending, rejected, suspended, and zero-stock pharmacies are excluded |
| TC-004 | Search result freshness | Freshness and availability reflect persisted confirmation time |
| TC-005 | Radius partitions | 0 uses the 5000 m default; 499 and 50001 are rejected; 500 and 50000 are accepted; negative is rejected |
| TC-006 | Search input validation | Empty query, out-of-range coordinates, and `max_offers` below 0 return `INVALID_ARGUMENT` |
| TC-007 | Ranking decision table | Distance, freshness, accuracy, and price tie-breakers are deterministic |
| TC-008 | Paid-placement invariant | No non-approved field or flag changes ranking order |
| TC-009 | Result details | Pharmacy, stock, price, distance, and freshness are visible and consistent |
| TC-010 | Create hold | Quantity 1–5 accepted, 0 and 6 rejected; stock is reserved and a unique pickup code is issued once the hold is ACTIVE |
| TC-011 | Hold boundary duration | With an injected clock (`RunSweep.as_of`): just before expiry stays ACTIVE, at and after expiry becomes EXPIRED |
| TC-012 | Concurrent holds | Inventory never goes below zero and only eligible requests succeed |
| TC-013 | Idempotent hold retry | Replayed request/event does not create a second hold or deduction |
| TC-014 | Collect with code | Valid code collects once; invalid, reused, and unauthorized codes fail |
| TC-015 | Expire hold | Scheduled expiry transitions state and releases stock |
| TC-016 | Cancel hold | Cancellation is legal only in allowed states and releases stock once |
| TC-017 | Stock synchronization | Pharmacy update persists and publishes the expected event |
| TC-018 | Search after stock update | Search reflects update after the documented propagation window |
| TC-019 | Freshness decay | Scheduled worker applies the correct decay at boundary times |
| TC-020 | Stale stock behavior | Stale/expired stock follows the visibility rule and is labelled |
| TC-021 | Substitution decision table | Table-driven tests cover molecule, strength, form, prescription-only, controlled, and source-product rules; see [the decision table](substitution-decision-table.md) |
| TC-022 | No substitution | No-match response is explicit and does not invent products |
| TC-023 | Watchlist lifecycle | Create, duplicate, remove, and trigger transitions are correct |
| TC-024 | Return-stock notification | Eligible watcher receives one notification and retries are safe |
| TC-025 | Pharmacy verification | Registry status gates listing and stock operations |
| TC-026 | Pharmacy dashboard stock | Authorized dashboard user can add/update/remove stock |
| TC-027 | Authentication states | Valid, expired, malformed, tampered, wrong-type, wrong-issuer, and revoked (rotated or logged out) tokens behave correctly; password length 7/8/72/73; a database outage is `INTERNAL`, never `UNAUTHENTICATED` |
| TC-028 | Role authorization | Patient, pharmacy, and unauthenticated callers receive least-privilege access |
| TC-029 | Gateway validation | REST input, status codes, error body, and correlation ID are correct |
| TC-030 | Gateway gRPC mapping | Correct method, timeout, and downstream error translation are verified |
| TC-031 | Saga failure/retry | Duplicate, delayed, rejected, and redelivered events converge safely |
| TC-032 | RabbitMQ contract | Routing key, schema, correlation ID, and consumer acknowledgement are correct |
| TC-033 | Feedback/accuracy | Feedback updates only approved trust inputs and is auditable |
| TC-034 | Observability | Success and failure paths emit searchable structured evidence |
| TC-035 | Search load profile | k6 latency, throughput, error rate, and resource thresholds pass |
| TC-036 | Reservation load profile | Concurrent holds preserve correctness under peak profile |
| TC-037 | Patient accessibility | axe, keyboard, focus, labels, contrast, and announcements pass |
| TC-038 | Pharmacy accessibility | Dashboard critical paths pass the same accessibility checks |
| TC-039 | Contract compatibility | Existing clients tolerate additive changes and migrations are safe |
| TC-040 | Test-data hygiene | No real secrets/patient data enter fixtures, logs, or artifacts |
| TC-041 | Pharmacy sign-up and approval | A new pharmacy is pending and off the map; after an admin approves it, it is on the map and its portal opens (SCRUM-155) |
| TC-042 | Pharmacy rejection | A rejected pharmacy sees the admin's reason and stays off the map; a rejected pharmacy cannot be approved (SCRUM-155) |

### 7.2 Automation status at the end of Sprint 1

Only Gateway, Auth, and Pricing exist as running services so far. Everything
not listed is planned for the sprint that builds the service. The TC-027 to
TC-029 rows include the Sprint 1 cleanup (SCRUM-106 auth, SCRUM-107 gateway),
and `make coverage` comes from SCRUM-89.

| Test case / area | Automated now | Where |
|---|---|---|
| TC-027 Authentication states | Yes (UT) | `services/auth/internal/{domain,app,adapters/grpc}` |
| TC-028 Role authorization | Partly (UT: role mapping, admin self-registration blocked) | `services/auth/internal/adapters/grpc` |
| TC-029 Gateway validation | Partly (UT: config, request ID, CORS, panic → 500) | `services/gateway/internal/{app,httpx}` |
| TC-030 Gateway gRPC mapping | Partly (UT: gRPC → HTTP status mapping, client factory) | `services/gateway/internal/{httpx,grpcx}` |
| TC-031 Saga retry / duplicates | Partly (UT: consumer dedupe, retry, dead-letter) | `pkg/messaging` |
| TC-032 RabbitMQ contract | Partly (IT: publish and consume a real event) | `pkg/messaging` (integration) |
| TC-039 Migration safety | Yes (IT: ordered, once only, rollback on failure, concurrent replicas) | `pkg/migrate` (integration) |
| Resilience patterns | Yes (UT: breaker, retry with backoff, timeout, bulkhead) | `pkg/resilience` |
| Pricing rules | Yes (UT: price boundaries 0/1, all-or-nothing updates) | `services/pricing/internal` |
| TC-007 Ranking decision table | Yes (UT: normalisation, median for unknown price, worked example, tie order) | `services/ranking/internal/domain` |
| TC-008 Paid-placement invariant | Yes (UT: order independent of input order; improving any factor never lowers rank; out of stock never ranked) | `services/ranking/internal/domain` |

### 7.3 Mutation testing results

Run with [Gremlins](https://gremlins.dev) v0.5.0, all mutators enabled
(`services/<name>/.gremlins.yaml`). From the repository root, `make mutation`
(`scripts/mutation-tests.sh`) runs every package below and prints this table;
`MUTATION_TARGETS="holds/internal/app" ./scripts/mutation-tests.sh` runs one. From a
service folder: `GOWORK=off gremlins unleash ./internal/domain` (or `./internal/app`).
Install Gremlins with `go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0`.
*Efficacy* (the mutation score) = killed ÷ (killed + lived); *mutant coverage* =
mutants on tested code ÷ all mutants. Each `.gremlins.yaml` sets the gate:
Ranking and Inventory fail below 95% for either, Holds below 90% efficacy or 60%
mutant coverage (see the Holds domain note below). SCRUM-113 asks for at least 70%
efficacy on Inventory and Holds.

| Package | Date | Killed | Lived | Not covered | Efficacy | Mutant coverage | Statement coverage |
|---|---|---|---|---|---|---|---|
| `services/ranking/internal/domain` (SCRUM-51) | 2026-10-30 | 55 | 0 | 0 | **100%** | **100%** | 100% |
| `services/inventory/internal/domain` (SCRUM-113) | 2026-11-12 | 36 | 0 | 0 | **100%** | **100%** | 100% |
| `services/inventory/internal/app` (SCRUM-113) | 2026-11-12 | 21 | 0 | 0 | **100%** | **100%** | 100% |
| `services/holds/internal/domain` (SCRUM-113) | 2026-11-12 | 20 | 1 | 13 | **95.2%** | 61.8% | 100% |
| `services/holds/internal/app` (SCRUM-113) | 2026-11-12 | 57 | 1 | 1 | **98.3%** | 98.3% | 97.5% |

Per service (domain and app together): **Inventory 100%** (57 of 57 killed) and
**Holds 97.5%** (77 killed, 2 lived), both above the 70% SCRUM-113 asks for.

**SCRUM-113: Inventory and Holds.** The first run gave:

| Package | Killed | Lived | Not covered | Efficacy | Mutant coverage |
|---|---|---|---|---|---|
| `inventory/internal/domain` | 28 | 8 | 0 | 77.8% | 100% |
| `inventory/internal/app` | 19 | 2 | 0 | 90.5% | 100% |
| `holds/internal/domain` | 20 | 1 | 13 | 95.2% | 61.8% |
| `holds/internal/app` | 55 | 3 | 1 | 94.8% | 98.3% |

How it was improved (tests only, no production code changed):

- **Inventory stock rules: 8 lived, all `<` → `<=` or `>` → `>=`.** No test sat
  exactly on a limit. `stock_boundary_test.go` now does: an empty shelf (0 on hand,
  0 reserved), everything reserved (reserved = on hand), setting on-hand to exactly
  the reserved quantity, reserving the last unit (available = 1) and one more than
  that, reserving with nothing available (available = 0), reserving exactly what is
  available and one more, and committing or releasing exactly the reserved quantity
  and one more. A lifecycle test checks that stock is never oversold.
- **Inventory bulk update: 2 lived.** `newAvailable <= previous` → `<` and `continue`
  → `break`. The test had no unchanged row and no row after a skipped one.
  `TestBulkSetSkipsUnchangedRowsAndKeepsGoing` has an unchanged row and a decreased
  row before two increased ones, and checks the exact `stock.increased` events.
- **Holds app: `newUUID` (2 lived).** Flipping a bit mask (`&` ↔ `|`) in the UUID
  version/variant bits was not caught. `TestNewUUIDMakesDistinctVersion4UUIDs`
  checks 200 IDs against the version 4 UUID pattern.

What is left, and why it is accepted:

- **Holds domain, 13 not covered: the checks in `NewHold` and `Collect`
  (`hold.go`).** They are `case` expressions in a tagless `switch`, which Go's
  coverage profile does not record, so Gremlins skips them although the table
  tests (`TestNewHoldValidates`, `TestCollect`, the state-transition table) run
  every case. Rewriting them as `if` statements (as Ranking did in SCRUM-51) made
  all 13 visible and killed, but it is a production change with no behaviour
  change, so it was left out of this test-only ticket. This is why the Holds gate
  for mutant coverage is 60%.
- **Lived, equivalent: `stored == "" || given == ""` → `&&` in
  `PickupCodeMatches` (`pickup.go:50`).** With `&&`, a pair where only one code
  is empty goes on to `subtle.ConstantTimeCompare`, which returns 0 for strings
  of different length, so the result is still "no match". No input can tell
  the two apart.
- **Lived, equivalent: `size > MaxPageSize` → `>=` in `ListHolds`
  (`service.go:168`).** When the size is exactly 100 it is set to 100, so nothing
  changes.
- **Not covered: `DefaultHoldDuration = 2 * time.Hour` (`service.go:19`).** A
  constant declaration is not a statement, so no coverage profile can include it.
  `TestDefaultsMatchTheRules` checks the value, so a change still fails a test.

**SCRUM-51: Ranking.** How the first run (46 killed, 1 lived, 8 not covered; 97.9%
efficacy) was brought to 100%:

- **Lived: `>` → `>=` in the weight-sum check.** No test sat exactly on the
  tolerance, and with a decimal tolerance (1e-6) no float sum can. The tolerance is
  now 2⁻²⁰, an exact binary fraction, and `TestWeightSumToleranceBoundary` checks
  sums exactly on it (valid) and just past it (invalid).
- **Not covered: the candidate checks.** They were `case` expressions in a tagless
  `switch`, which Go's coverage profile does not record, so Gremlins could not see
  that the tests reach them. The checks are now plain `if` statements; behaviour is
  unchanged and every one is killed.
- **Not covered: the tolerance constant.** Gremlins mutated the operators in
  `1.0 / (1 << 20)`, which no coverage profile can include. It is now the literal
  `0x1p-20` (the same value).

### 7.4 Automation status at the end of Sprint 4

UAT results and participant sign-off dispositions are recorded in
[docs/uat.md](uat.md); any formal SRS changes require Product Owner approval.

Sprint 4 built the reservation path: the Holds saga (SCRUM-61), Inventory's saga
events (SCRUM-57), Expiry (SCRUM-62), Feedback (SCRUM-64), Watchlist (SCRUM-59), the
gateway reservation routes (SCRUM-54) and the patient reserve screens (SCRUM-60).
This table adds to 7.2; rows there still hold.

At the end of Sprint 4, CI ran two jobs on every PR: **Go checks** (vet,
golangci-lint, unit tests and the coverage gate for every module) and **Saga
integration tests** (SCRUM-56: `tests/integration`, Postgres and RabbitMQ in
testcontainers, the real Holds and Inventory built with `-race`). Sprint 5 adds
`-race` to the unit tests and three more jobs (SCRUM-116, see section 4).

| Test case / area | Automated now | Where |
|---|---|---|
| TC-001 / TC-003 Search and availability | Yes (Compose: seeded product and available pharmacy offer are returned through the gateway) | `tests/integration/compose` |
| TC-010 Create hold | Yes (UT: quantity 0 and 6 rejected at the gateway; IT: `PENDING` → `ACTIVE` with a pickup code, stock reserved, `hold.confirmed` sent; Compose: gateway reservation) | `services/gateway/internal/app` (`TestHoldHandlerCreateRejectsOutOfRangeQuantity`), `tests/integration/saga`, `tests/integration/compose` |
| TC-011 Hold boundary duration | Yes (UT: a hold expiring exactly now is expired, injected clock; IT: an early `ExpireHold` is refused and keeps the stock; Compose: gateway-triggered expiry sweep) | `services/expiry/internal/app` (`TestAHoldExpiringExactlyNowIsExpired`, `TestHoldBoundaryWithAnInjectedClock`), `tests/integration/saga`, `tests/integration/compose` |
| TC-012 Concurrent holds | Yes (IT: exactly one winner for the last unit; stock never negative under reserve and release) | `services/inventory/internal/adapters/postgres` (integration, SCRUM-37) |
| TC-013 Idempotent hold retry | Yes (UT: the same event twice, and the same answer under a new event ID, change nothing; IT: replayed `hold.requested` and `stock.reserved` leave one reservation) | `services/holds/internal/adapters/rabbitmq`, `services/inventory/internal/adapters/rabbitmq` (`TestDuplicateHoldRequestedDeliveryReservesOnce`), `tests/integration/saga` |
| TC-014 Collect with code | Yes (UT: wrong code refused, collecting twice fails, stock committed before the hold changes) | `services/holds/internal/{domain,app}` |
| TC-015 Expire hold | Yes (UT: a sweep run twice expires each hold once; IT: expiry releases the stock, a second call changes nothing) | `services/expiry/internal/app`, `tests/integration/saga` |
| TC-016 Cancel hold | Yes (UT: cancelling twice releases once, another patient never reaches Inventory; IT: cancel releases the stock) | `services/holds/internal/app`, `tests/integration/saga` |
| TC-023 Watchlist lifecycle | Yes (UT: subscribe, duplicate returns the first, unsubscribe; IT: the same contract against Postgres) | `services/watchlist/internal/{domain,app,adapters}` |
| TC-024 Return-stock notification | Yes (UT: `stock.increased` alerts only nearby subscribers, each once; Compose: gateway subscription and SSE alert after the real RabbitMQ event) | `services/watchlist/internal/app`, `tests/integration/compose` |
| TC-028 Role authorization | Partly (UT: missing token 401, wrong role 403, identity added to the request) | `services/gateway/internal/httpx` |
| TC-031 Saga failure/retry | Yes (UT: each event path, duplicates, retries; IT: rejected and duplicate paths end to end) | `services/holds/internal/adapters/rabbitmq`, `tests/integration/saga` |
| TC-032 RabbitMQ contract | Yes (IT: the saga's routing keys and payloads observed on `swiftmed.events`) | `tests/integration/saga` |
| TC-033 Feedback/accuracy | Yes (UT: only a collected hold, once per hold, one `feedback.submitted`; Compose: collected hold, gateway feedback, and trust score observed in search) | `services/feedback/internal/app`, `tests/integration/compose` |
| TC-035 / TC-036 Load profiles | Scripts only (k6 search and reservation burst with thresholds in the scripts). Results are not recorded yet | `tests/load` (SCRUM-77) |
| TC-037 Patient accessibility | Partly (UT: axe on the search, results, hold, sign-in and reserve components) | `web/swiftmed/components` |

Not yet covered in Sprint 4: TC-026 and TC-038 (pharmacy dashboard: SCRUM-58 and
SCRUM-70 carried over to Sprint 5).

### 7.5 Unit test coverage of Catalog, Stock Sync and User Profile (SCRUM-113)

These three services had one or two test files each. SCRUM-113 added behaviour tests
for their domain rules, use cases and gRPC adapters (Stock Sync's gRPC clients too).
Statement coverage before (`develop` at `71c5e0d`) and after, measured with
`GOWORK=off go test -race -count=1 -cover ./...` per package and
`./scripts/coverage-gate.sh` for the gate column:

| Service | Test files | Gate (domain + app) | `internal/...` | Whole module |
|---|---|---|---|---|
| Catalog | 1 → 4 | 80.6% → **100%** | 21.7% → **57.8%** | 18.6% → **49.7%** |
| Stock Sync | 2 → 6 | 77.6% → **98.3%** | 41.5% → **77.2%** | 35.8% → **66.6%** |
| User Profile | 2 → 5 | 93.2% → **97.7%** | 49.4% → **73.5%** | 33.3% → **49.6%** |

| Package | Before | After | What the new tests check |
|---|---|---|---|
| `catalog/internal/domain` | 0% | 100% | Every required field, blank input, pack size 0/1, every dosage form and unknown ones, strength normalization (`" 2.5 MG / 5 mL "` → `2.5mg/5ml`) |
| `catalog/internal/app` | 86.0% | 100% | Search limit −5/0/1/99/100/101, blank IDs never reach the repository, duplicate IDs removed in first-seen order, repository errors wrapped |
| `catalog/internal/adapters/grpc` | 0% | 98.7% | Every field mapped, NOT_FOUND / INVALID_ARGUMENT / INTERNAL (without leaking the cause), nil requests, dosage forms both ways |
| `stock-sync/internal/domain` | 92.8% | 98.6% | BOM, spaces and blank lines, price 0/1/blank/decimal/overflow, quantity 0 to 2³¹−1 and overflow, column count, header variants, malformed CSV, the 1 MB limit exactly and one byte over |
| `stock-sync/internal/app` | 67.6% | 98.1% | A mixed file (unknown, unparsable, Inventory-rejected and applied rows, errors sorted by row), Catalog, Inventory or Pricing down, page size 0/1/100/101, trimmed input, UTC times |
| `stock-sync/internal/adapters/grpc` | 0% | 100% | Job and row-error mapping, status codes (upstream gRPC status kept, internal causes hidden), default page |
| `stock-sync/internal/adapters/clients` | 0% | 91.7% | Requests sent to Catalog, Inventory and Pricing, nil entries skipped, call errors keep their gRPC status |
| `user-profile/internal/domain` | 100% | 100% | Phone length 7/8/15/16 digits, leading zero, `+`, spaces; `Validate` agrees with `NewProfile` |
| `user-profile/internal/app` | 88.9% | 96.3% | `UpdateProfile` keeps `created_at` and moves `updated_at`, unknown user, invalid input changes nothing, duplicate create |
| `user-profile/internal/adapters/grpc` | 0% | 100% | Create-or-update, NOT_FOUND, INVALID_ARGUMENT, ALREADY_EXISTS on a create race, INTERNAL without the cause |

Still at 0%: `adapters/postgres` (needs a database; these services have no
integration tests yet) and `cmd/server` (wiring only). The coverage gate passes;
these three services are still report-only in it.

## 8. Risk-based integration order

1. **P0 foundation:** protobuf/event contracts, configuration, logging,
   authentication, gateway validation, and database migrations.
2. **P0 inventory truth:** Catalog, Inventory, Stock Sync, Freshness, and
   Pharmacy Registry; verify non-negative stock and stale-data behavior.
3. **P0 reservation saga:** Holds, Expiry, Inventory, RabbitMQ, and pickup
   collection; prioritize concurrency, retries, duplicate events, and
   rollback.
4. **P0 discovery:** Geo, Search, Ranking, and Pricing; verify location,
   freshness, deterministic tie-breaking, and the no-paid-placement invariant.
5. **P0 user surfaces:** patient application and pharmacy dashboard over the
   gateway, including authorization and accessibility.
6. **P1 recovery and engagement:** Substitution, Watchlist, Notification,
   Feedback, Trust, and Analytics.
7. **Non-functional qualification:** k6 load/soak, mutation testing, resilience
   fault injection, full accessibility, and UAT.

This order minimizes the chance that a UI result masks an incorrect stock,
identity, or reservation state and gives the highest-risk integration paths the
most stabilization time.

## 9. Reporting and maintenance

Each run records commit SHA, environment, seed version, test level, case IDs,
result, duration, logs/artifacts, and defect references. The QA lead owns the
matrix. Service owners update unit and integration cases with behavior changes;
the product owner approves UAT changes. Any new SRS requirement, service,
endpoint, event, or user journey must add a matrix row and test case before it
is considered complete.
