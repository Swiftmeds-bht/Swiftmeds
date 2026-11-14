# Feature files (BDD)

Given/When/Then scenarios for the core patient and pharmacy journeys (SCRUM-112).
They are written from the requirements in [docs/test-plan.md](../../docs/test-plan.md)
section 7, which is the SRS baseline until a formal SRS is committed. The Cypress
specs (SCRUM-72) are built from these scenarios.

| File | User story | Requirements |
|---|---|---|
| [search_medicine.feature](search_medicine.feature) | Search for a medicine nearby: in stock, out of stock, substitution offered | SRS-F-01, 02, 03, 04, 05, 11, 12, 16 |
| [reserve_and_collect.feature](reserve_and_collect.feature) | Reserve, get a pickup code, collect | SRS-F-06, 07, 08, 09, NFR-01 |
| [reservation_rejected_and_expired.feature](reservation_rejected_and_expired.feature) | Reservation rejected (no stock), hold expired (stock released) | SRS-F-06, 07, 08, 09, 17 |
| [watchlist_restock_alert.feature](watchlist_restock_alert.feature) | Watch a medicine, get an alert on restock | SRS-F-13 |
| [feedback_trust_score.feature](feedback_trust_score.feature) | Feedback after pickup updates the trust score | SRS-F-18, 04, 15 |
| [pharmacy_stock.feature](pharmacy_stock.feature) | Pharmacy updates stock and collects holds from the dashboard | SRS-F-08, 10, 14 |
| [account_access.feature](account_access.feature) | Sign-in, sessions and role checks | SRS-F-15 |

47 scenarios, 61 test cases when every `Examples` row is counted.

## Tags

Every scenario carries three kinds of tag, used to build the traceability matrix:

- `@SRS-F-xx` / `@SRS-NFR-xx`: the requirement it checks (one or more).
- `@TC-xxx`: the test case in the register (test-plan section 7.1).
- `@must` or `@should`: the requirement's priority. `@must` is P0, `@should` is P1.

Each file also has one area tag at the top (`@search`, `@reservation`, `@watchlist`,
`@feedback`, `@pharmacy`, `@auth`), so one area can be run on its own, for example
`--env tags="@reservation and @must"` with the Cypress cucumber preprocessor.

## Coverage

Every MUST (P0) functional requirement and every P1 functional requirement has at
least one scenario:

| Requirement | Priority | Scenarios in |
|---|---|---|
| SRS-F-01 search by product, molecule, strength | P0 | search_medicine |
| SRS-F-02 verified pharmacies with stock only | P0 | search_medicine |
| SRS-F-03 location and radius checks | P0 | search_medicine |
| SRS-F-04 ranking factors | P0 | search_medicine, feedback_trust_score |
| SRS-F-05 result details | P1 | search_medicine |
| SRS-F-06 reserve and get a pickup code | P0 | reserve_and_collect, reservation_rejected_and_expired |
| SRS-F-07 no overselling, idempotent | P0 | reserve_and_collect, reservation_rejected_and_expired |
| SRS-F-08 collect once with a valid code | P0 | reserve_and_collect, reservation_rejected_and_expired, pharmacy_stock |
| SRS-F-09 expired or cancelled holds release stock | P0 | reserve_and_collect, reservation_rejected_and_expired |
| SRS-F-10 stock updates reach search | P0 | pharmacy_stock |
| SRS-F-11 stale stock labelled | P0 | search_medicine |
| SRS-F-12 substitution | P1 | search_medicine |
| SRS-F-13 watchlist alert | P1 | watchlist_restock_alert |
| SRS-F-14 verified pharmacies, dashboard stock | P0 | pharmacy_stock |
| SRS-F-15 authentication and roles | P0 | account_access, feedback_trust_score |
| SRS-F-16 gateway validation | P0 | search_medicine |
| SRS-F-17 events, retries, duplicates | P0 | reservation_rejected_and_expired |
| SRS-F-18 feedback and trust | P1 | feedback_trust_score |
| SRS-NFR-01 atomic, never negative stock | P0 | reserve_and_collect |

The other non-functional requirements are not user stories, so they have no
scenario here. They are checked at their own test level instead:

| Requirement | Checked by |
|---|---|
| SRS-NFR-02 observability | integration tests (TC-034) |
| SRS-NFR-03 search latency under load | k6 in [tests/load](../load) (TC-035, TC-036) |
| SRS-NFR-04 accessibility | axe in Cypress plus manual checks (TC-037, TC-038) |
| SRS-NFR-05 contract compatibility | contract and migration tests (TC-039) |
| SRS-NFR-06 test-data hygiene | review and integration tests (TC-040) |

## Writing the Cypress steps (SCRUM-72)

- The same sentence means the same thing in every file, so one step definition
  serves all of them. For example `I am signed in as a patient`,
  `"<pharmacy>" has <n> packs of "<product>"`, `I reserve <n> packs of "<product>" at "<pharmacy>"`,
  `my hold is "<status>"` and `I search for "<product>" within 5 km of Thimphu`.
- Quoted UI text matches the apps today, for example "Ready to collect",
  "Not available", "Expired", "Notify me when it is back",
  "Same medicine, other brands" and "Collect hold". If the wording in the app
  changes, change the scenario too.
- `Given` steps are data setup. Seed them through the gateway API (`cy.request`)
  or the seed scripts, not through the UI, so each scenario starts clean.
- Some steps have no screen yet. Feedback has no page in the patient app, and the
  trust score and event deliveries are not visible in any UI. Drive those through
  the gateway API (`POST /api/v1/holds/{holdId}/feedback`) and check the result
  through the API or the service under test.
- Expiry scenarios should not wait 2 hours. Run the sweep with an `asOf` time in
  the future (`POST /api/v1/internal/expiry/sweep`), as TC-011 describes.
- Pharmacy names, products and prices are made-up test data. Never use real
  patient data (SRS-NFR-06).
