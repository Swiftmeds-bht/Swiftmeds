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

- Clinical advice, diagnosis, prescription validation, or dispensing decisions.
- Payment processing and delivery logistics, which are not specified by the
  current product baseline.
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
- **Boundary-value analysis:** zero/negative/maximum radius, empty and maximum
  result sets, stock `0/1/max`, hold duration at just before/at/just after
  expiry, freshness thresholds, pagination limits, and k6 rate thresholds.
- **Decision tables:** reservation eligibility (verified pharmacy, available
  stock, active user, duplicate hold, and concurrent requests), search ranking
  tie-breakers, substitution matching, and notification eligibility.
- **State-transition testing:** stock confirmed/stale/expired, hold
  pending/confirmed/collected/expired/cancelled, user authenticated/expired/
  unauthorized, and watchlist inactive/triggered/delivered.
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
| Go unit and integration tests | `go test ./...` per Go module; repository `make test` |
| Static quality gate | `make lint` and the CI configuration |
| Service dependencies | Docker Compose with Postgres, RabbitMQ, and Traefik |
| Browser E2E and fixtures | Cypress; scenarios under `tests/e2e` and `tests/features` |
| Load and soak | k6 scripts under `tests/load`; collect latency, errors, throughput, queue lag, and resource use |
| Accessibility | axe in Cypress plus manual keyboard and screen-reader checks |
| Mutation testing | Gremlins, scoped first to core domain packages |
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
| Load | No threshold breach for p95 latency, error rate, throughput, or queue lag defined by the release profile |
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
| SRS-F-14 | Only registered and verified pharmacies contribute eligible listings, and pharmacy users can manage their stock through the dashboard. | TC-025, TC-026 | IT, E2E, UAT | P0 |
| SRS-F-15 | Authentication, token expiry, roles, and authorization prevent patients and pharmacy users from accessing each other's protected operations. | TC-027, TC-028 | UT, IT, E2E | P0 |
| SRS-F-16 | Gateway REST requests are validated and translated to the correct gRPC operation and documented error response. | TC-029, TC-030 | IT, Contract | P0 |
| SRS-F-17 | Services exchange reservation and stock events through RabbitMQ with correlation, retry, duplicate, and failure handling. | TC-031, TC-032 | IT, Failure injection | P0 |
| SRS-F-18 | Analytics and feedback record the agreed pharmacy accuracy and user feedback data without changing the ranking inputs outside their defined rules. | TC-033 | IT, UAT | P1 |
| SRS-NFR-01 | Core reservation and inventory operations preserve atomicity, consistency, and non-negative stock across retries and failures. | TC-012, TC-013, TC-031 | UT, IT, Mutation | P0 |
| SRS-NFR-02 | The system remains observable through structured logs, correlation IDs, event outcomes, and actionable errors. | TC-034 | IT, UAT | P0 |
| SRS-NFR-03 | Critical gateway and search flows meet the release k6 latency, throughput, error-rate, and queue-lag thresholds. | TC-035, TC-036 | LT | P0 |
| SRS-NFR-04 | Patient and pharmacy web journeys meet WCAG-oriented keyboard, focus, naming, contrast, and status-announcement checks. | TC-037, TC-038 | AT, E2E | P0 |
| SRS-NFR-05 | Public contracts, migrations, and generated protobuf code remain backward compatible for supported clients and services. | TC-039 | Contract, IT, Regression | P1 |
| SRS-NFR-06 | Test environments isolate data, use non-production credentials, and never expose secrets or real patient data. | TC-040 | IT, UAT, Review | P0 |

### 7.1 Test case register

| ID | Short description | Main assertions |
|---|---|---|
| TC-001 | Valid product search | Matching product and strength are returned |
| TC-002 | Molecule/strength equivalence | Equivalent molecule search returns intended catalog entries |
| TC-003 | Verified pharmacy availability | Unverified, closed, and zero-stock pharmacies are excluded |
| TC-004 | Search result freshness | Freshness and availability reflect persisted confirmation time |
| TC-005 | Radius partitions | Empty, negative, zero, normal, and maximum radius are handled |
| TC-006 | Search input validation | Malformed coordinates, strength, and pagination return documented errors |
| TC-007 | Ranking decision table | Distance, freshness, accuracy, and price tie-breakers are deterministic |
| TC-008 | Paid-placement invariant | No non-approved field or flag changes ranking order |
| TC-009 | Result details | Pharmacy, stock, price, distance, and freshness are visible and consistent |
| TC-010 | Create hold | Eligible stock decreases/reserves and a unique pickup code is issued |
| TC-011 | Hold boundary duration | Before, at, and after expiry have the specified outcome |
| TC-012 | Concurrent holds | Inventory never goes below zero and only eligible requests succeed |
| TC-013 | Idempotent hold retry | Replayed request/event does not create a second hold or deduction |
| TC-014 | Collect with code | Valid code collects once; invalid, reused, and unauthorized codes fail |
| TC-015 | Expire hold | Scheduled expiry transitions state and releases stock |
| TC-016 | Cancel hold | Cancellation is legal only in allowed states and releases stock once |
| TC-017 | Stock synchronization | Pharmacy update persists and publishes the expected event |
| TC-018 | Search after stock update | Search reflects update after the documented propagation window |
| TC-019 | Freshness decay | Scheduled worker applies the correct decay at boundary times |
| TC-020 | Stale stock behavior | Stale/expired stock follows the visibility rule and is labelled |
| TC-021 | Substitution matching | Same molecule and strength suggestions exclude unsafe mismatches |
| TC-022 | No substitution | No-match response is explicit and does not invent products |
| TC-023 | Watchlist lifecycle | Create, duplicate, remove, and trigger transitions are correct |
| TC-024 | Return-stock notification | Eligible watcher receives one notification and retries are safe |
| TC-025 | Pharmacy verification | Registry status gates listing and stock operations |
| TC-026 | Pharmacy dashboard stock | Authorized dashboard user can add/update/remove stock |
| TC-027 | Authentication states | Valid, expired, missing, malformed, and revoked tokens behave correctly |
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
