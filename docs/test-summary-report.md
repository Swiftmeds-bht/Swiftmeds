# SwiftMed Test Summary Report (SCRUM-76)

**Report date:** 2026-11-18  
**Branch:** `docs/SCRUM-76-test-summary-report`  
**Baseline:** `docs/test-plan.md`, section 7 (40 requirements; no separate SRS is present)  
**Result:** **Not release-qualified.** Each baseline requirement is mapped below. The repository records executed evidence for some cases, but several mapped cases have scripts or plans only, and no single run artifact establishes a passing result for every requirement.

## Summary by test level

| Level | Recorded result | Evidence / limitation |
|---|---|---|
| Unit | Pass evidence exists for the named service and package suites below; this report does not claim a fresh full-suite run. | CI runs Go tests with race detection and coverage; documented Sprint 1 and Sprint 4 cases and SCRUM-113 coverage results are cited in the matrix. Patient app and worker suites are configured in CI; no run-specific artifact is checked in. |
| Integration | Pass evidence exists for reservation saga, selected repository, messaging, and migration cases. | `tests/integration` exercises Holds, Inventory, Postgres and RabbitMQ. `scripts/integration-tests.sh` and CI define broader suites. The committed documents do not include a current CI run URL or complete per-case result export. |
| E2E | Partial evidence only. | Saga integration exercises service-to-service flows. Cypress feature scenarios are mapped in `tests/features`, but the docs describe Cypress specs as built from these scenarios; no Cypress run result/artifact is recorded. |
| Load | Not executed / no result recorded. | `tests/load` defines k6 search and reservation profiles and thresholds. Its results table is still `_pending_`; reservation burst also records a dependency on gateway routes. |
| Accessibility | Partial automated unit evidence; full qualification not recorded. | Patient app axe tests are present. Manual keyboard/screen-reader checks and pharmacy-dashboard checks have no recorded results. |
| UAT | Not recorded. | No signed scenario checklist or product-owner acceptance evidence is present. |

## Coverage and mutation results

The latest committed, dated coverage evidence is from SCRUM-113. These are test-run results recorded in `docs/test-plan.md`, not a new run for this report.

| Area | Statement coverage recorded | Result |
|---|---:|---|
| Catalog domain + app | 100% | Above the 80% core gate; adapters and whole-module totals are lower (57.8% internal, 49.7% whole module). |
| Stock Sync domain + app | 98.3% | Above the 80% core gate; 77.2% internal, 66.6% whole module. |
| User Profile domain + app | 97.7% | Above the 80% core gate; 73.5% internal, 49.6% whole module. |
| Ranking domain | 100% | Recorded with mutation run. |
| Inventory domain / app | 100% / 100% | Recorded with mutation run. |
| Holds domain / app | 100% / 97.5% | Recorded with mutation run. |

Mutation efficacy / mutant coverage recorded: Ranking 100% / 100%; Inventory domain and app 100% / 100%; Holds domain 95.2% / 61.8%; Holds app 98.3% / 98.3%. The surviving mutants are documented as equivalent, and 13 Holds domain mutants are not covered because Go coverage does not expose tagless-switch case expressions; the exercised table cases are described in the test plan. Inventory combined efficacy is 100%; Holds combined efficacy is 97.5%. These results exceed the documented SCRUM-113 efficacy target, with Holds domain mutant coverage accepted at its configured 60% gate.

The repository defines an 80% coverage gate for core services Inventory, Holds, Ranking, Freshness, and Substitution, enforced by `scripts/coverage-gate.sh` in CI. This report does not invent current percentages for services without committed numeric results.

## Traceability matrix and executed evidence

“Executed evidence” below refers only to an execution/result explicitly recorded in the committed test-plan history or in a named automated test artifact. A test definition, CI job configuration, or threshold script alone is not counted as a completed run. Requirements with no named executed evidence remain open against the SCRUM-76 completion criterion.

| Requirement | Test case(s) | Executed evidence recorded | Status |
|---|---|---|---|
| SRS-F-01 | TC-001, TC-002 | No case-specific execution result recorded. | Mapped; execution unverified |
| SRS-F-02 | TC-003, TC-004 | No case-specific execution result recorded. | Mapped; execution unverified |
| SRS-F-03 | TC-005, TC-006 | TC-029 has partial gateway validation unit evidence; radius/search cases have no recorded run. | Partial |
| SRS-F-04 | TC-007, TC-008 | Ranking decision-table and paid-placement invariant unit tests are recorded as executed. | Executed |
| SRS-F-05 | TC-009 | No result-details execution result recorded. | Mapped; execution unverified |
| SRS-F-06 | TC-010, TC-011 | Gateway quantity-boundary unit test, expiry boundary unit tests, and saga integration evidence are recorded. | Executed |
| SRS-F-07 | TC-012, TC-013 | Inventory concurrent-hold integration and duplicate-event unit/integration evidence are recorded. | Executed |
| SRS-F-08 | TC-014 | Holds domain/app unit evidence checks invalid and repeated collection; a full journey result is not recorded. | Executed (unit); E2E unverified |
| SRS-F-09 | TC-015, TC-016 | Expiry/cancel unit and saga integration evidence are recorded, including repeated calls and stock release. | Executed |
| SRS-F-10 | TC-017, TC-018 | No case-specific execution result recorded. | Mapped; execution unverified |
| SRS-F-11 | TC-019, TC-020 | No case-specific execution result recorded. | Mapped; execution unverified |
| SRS-F-12 | TC-021, TC-022 | No case-specific execution result recorded. | Mapped; execution unverified |
| SRS-F-13 | TC-023, TC-024 | Watchlist lifecycle unit and Postgres integration evidence recorded; notification unit logic is partial and app delivery remains SCRUM-71. | Partial |
| SRS-F-14 | TC-025, TC-026 | Pharmacy registry/dashboard case results are not recorded. | Mapped; execution unverified |
| SRS-F-15 | TC-027, TC-028 | Authentication state unit tests and partial role/authorization unit tests are recorded. | Partial |
| SRS-F-16 | TC-029, TC-030 | Partial gateway validation and gRPC-to-HTTP mapping/client-factory unit results recorded. | Partial |
| SRS-F-17 | TC-031, TC-032 | Messaging retry/dedupe unit tests and saga RabbitMQ contract/replay integration evidence recorded. | Executed |
| SRS-F-18 | TC-033 | Feedback service unit evidence is partial; Trust integration is identified as follow-up, not recorded complete. | Partial |
| SRS-NFR-01 | TC-012, TC-013, TC-031 | Concurrent inventory and idempotent saga tests recorded; Inventory mutation testing killed all 57 mutants. | Executed |
| SRS-NFR-02 | TC-034 | No named observability execution result recorded. | Mapped; execution unverified |
| SRS-NFR-03 | TC-035, TC-036 | k6 scripts and thresholds exist, but results are explicitly pending. | Not executed / open |
| SRS-NFR-04 | TC-037, TC-038 | Patient app axe component unit tests recorded as partial; manual checks and dashboard suite are unreported. | Partial |
| SRS-NFR-05 | TC-039 | Migration integration tests are recorded as executed (ordered, once-only, rollback, concurrent replicas). | Executed (migration); contract compatibility unverified |
| SRS-NFR-06 | TC-040 | No completed review or test result recorded. | Mapped; execution unverified |

**Traceability coverage:** 24/24 baseline requirements have at least one mapped test case (100%). **Execution completion:** not demonstrated for all requirements. The done criterion “every SRS requirement maps to at least one executed test” is therefore **not yet met**; the open rows above identify where run evidence is needed. Once a formal SRS is added, its IDs and rows supersede this repository-derived baseline.

## Open defects and release gaps

No defect tracker export or defect IDs are committed with the test evidence, so there is no substantiated defect count. The following are documented test gaps or blockers, not asserted product defects:

- TC-035/TC-036: k6 results pending; reservation burst dependency was documented as SCRUM-115 and SCRUM-54. Scripts alone do not establish performance or no-oversell qualification at load.
- TC-026/TC-038: pharmacy dashboard behavior/accessibility were carried into Sprint 5 in the test plan; no execution result is recorded here.
- TC-024 and TC-033: notification delivery and Trust-side feedback integration were explicitly partial in the Sprint 4 status.
- TC-037: axe unit coverage exists for selected patient components, but full keyboard/screen-reader review is not recorded.
- UAT: no signed product-owner checklist or acceptance decision is recorded.
- Multiple functional requirements in the matrix have no case-specific executed result in the committed history. Attach CI/test artifacts or run the mapped tests before marking them complete.

## Evidence sources

- [Test plan, baseline requirements, case register, coverage and mutation history](test-plan.md)
- [BDD feature scenarios and requirement mapping](../tests/features/README.md)
- [Integration suite and reservation saga scenarios](../tests/integration/README.md)
- [k6 profiles and pending results](../tests/load/README.md)
- [CI job definitions](../.github/workflows/ci.yml)
