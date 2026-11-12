# Holds service

Short reservations of stock for a patient. A patient holds up to 5 packs of a medicine at one
pharmacy, collects them with a **pickup code**, and the hold ends in `COLLECTED`, `EXPIRED`,
`CANCELLED` or `REJECTED`.

## Contract

[`proto/holds/v1/holds.proto`](../../proto/holds/v1/holds.proto)

| RPC | What it does | Errors |
|---|---|---|
| `CreateHold` | Stores a `PENDING` hold (quantity 1 to 5) and publishes `hold.requested`, which starts the [reservation saga](#reservation-saga). | `INVALID_ARGUMENT`, `RESOURCE_EXHAUSTED` when the patient already has 3 open holds, `UNAVAILABLE` if the event could not be sent (the hold is then `REJECTED`; try again) |
| `GetHold` | Returns one hold. The patient's pickup code is included while the hold is `ACTIVE`. | `NOT_FOUND` |
| `ListHolds` | Holds of a patient (`user_id`) **or** of a pharmacy (`pharmacy_id`), newest first, optional status filter, paged. Pickup codes are included only in the patient's list. | `INVALID_ARGUMENT` |
| `CancelHold` | The patient who made the hold cancels it. An `ACTIVE` hold gives its stock back to Inventory first. | `PERMISSION_DENIED` (not their hold), `FAILED_PRECONDITION` (already finished), `UNAVAILABLE` (Inventory could not be reached; the hold is unchanged, try again) |
| `CollectHold` | Pharmacy staff hand over the medicine. Needs `hold_id`, `pharmacy_id` and the patient's `pickup_code`. | `PERMISSION_DENIED` (wrong code or pharmacy), `FAILED_PRECONDITION` (already collected, expired, cancelled ...), `ABORTED` (lost a race with another change), `UNAVAILABLE` (Inventory could not be reached; the hold stays `ACTIVE`, try again) |
| `ExpireHold` | Called by Expiry once `expires_at` has passed. Gives the stock back to Inventory first. Does nothing unless the hold is `ACTIVE`. | `FAILED_PRECONDITION` if called before `expires_at`, `UNAVAILABLE` (Inventory could not be reached; the hold stays `ACTIVE`, Expiry calls again) |

## Rules

- **State machine:** `PENDING -> ACTIVE | REJECTED | CANCELLED`, `ACTIVE -> COLLECTED | EXPIRED | CANCELLED`.
  Every other move is refused (`internal/domain/status.go`).
- **Pickup code:** 6 characters from `23456789ABCDEFGHJKLMNPQRSTUVWXYZ` (no `0 O 1 I`), made with
  `crypto/rand` when the hold becomes `ACTIVE`. It is unique among the `ACTIVE` holds of one pharmacy
  (a database unique index; a clash makes the service pick a new code). Codes are compared in constant
  time and typed input is forgiven (`abc-234` equals `ABC234`).
- **Collecting:** the hold must be `ACTIVE`, belong to the calling pharmacy, not be past `expires_at`,
  and the code must match. A wrong code is refused and the hold stays `ACTIVE`. Collecting twice
  fails because the second call finds a `COLLECTED` hold.
- **Open-hold limit:** a patient may have at most `HOLDS_MAX_ACTIVE_PER_USER` (default 3) holds that
  are `PENDING` or `ACTIVE`. The check and the insert are one database transaction under a per-patient
  advisory lock, so many `CreateHold` calls at once cannot get past the limit. Finished holds do not count.
- **Races:** every change is `UPDATE ... WHERE status = <the status that was read>`. If Collect and
  Expire (or two Collects) arrive together, one wins and the other gets `ABORTED`.
- **Pickup codes are private to the patient:** replies to pharmacy staff (`CollectHold`, `ExpireHold`,
  `CancelHold`, `ListHolds` by pharmacy) never include the code.

## Reservation saga

Holds and Inventory never call each other to make a reservation. They answer each other's events over
RabbitMQ (exchange `swiftmed.events`), and the hold's state shows where the saga is:

```
 patient            Holds                          RabbitMQ                    Inventory
   | CreateHold ----> | store PENDING                  |                           |
   |                  | -- hold.requested -----------> | ------------------------> | reserve stock
   |                  |                                | <-- stock.reserved ------ |   (or stock.unavailable)
   |                  | <------------------------------|                           |
   |                  | PENDING -> ACTIVE (pickup code, expiry time)               |
   |                  | -- hold.confirmed -----------> | (Notification, ...)       |
```

| Event | Hold is | What Holds does |
|---|---|---|
| `stock.reserved` | `PENDING` | `ACTIVE` with a pickup code and expiry time, then publishes `hold.confirmed` |
| `stock.reserved` | `ACTIVE` | A duplicate, or a retry after the publish failed: publishes `hold.confirmed` again. Nothing else changes. |
| `stock.reserved` | `CANCELLED`, `EXPIRED`, `REJECTED` | The patient cancelled while Inventory was reserving. The stock is released (`ReleaseReservation`). |
| `stock.reserved` | `COLLECTED` | Nothing (cannot happen: collecting needs an `ACTIVE` hold) |
| `stock.unavailable` | `PENDING` | `REJECTED`, reason `stock unavailable` |
| `stock.unavailable` | anything else | Nothing: the answer no longer applies |

**Compensation (giving stock back).** Inventory keeps one reservation per hold and allows only one of
release and commit, so Holds asks Inventory first and changes the hold second:

| Action | Inventory call, before the hold changes |
|---|---|
| `CancelHold` on an `ACTIVE` hold | `ReleaseReservation` |
| `CancelHold` on a `PENDING` hold | none: nothing is known to be reserved yet. If Inventory reserves a moment later, the `stock.reserved` row above releases it. |
| `ExpireHold` | `ReleaseReservation` |
| `CollectHold` | `CommitReservation`, only after the pickup code, pharmacy and expiry checks passed |

If the Inventory call fails, the hold stays as it was and the caller gets `UNAVAILABLE`. Both calls can be
repeated, so the patient (or the Expiry service) simply tries again, and stock is never left reserved
for a hold that has ended. (`CommitReservation` is not in the SCRUM-61 text, but without it collected
stock would stay "reserved" for ever.)

**Idempotent consumers.** Two layers:

1. `pkg/messaging` records each processed `event_id` in Postgres (`swiftmed_processed_events`) and
   acknowledges a repeat without running the handler. A failing event is retried 3 times, then
   dead-lettered (`holds.stock-events.dlq`).
2. The handlers are safe on their own, because the same answer can come back under a new `event_id`.
   Every row of the table above is a no-op or a repeat of something already done.

The events Holds sends carry an `event_id` made from the event kind and the hold ID, and their content
comes from the hold, not from the clock. Sending `hold.confirmed` twice therefore sends the same
message twice, and the listener drops the second one.

If `hold.requested` cannot be published, `CreateHold` rejects the hold at once and returns `UNAVAILABLE`.
Otherwise Inventory would never hear of it, and it would sit `PENDING` for ever and use up the
patient's limit.

## Known gaps

- **Crash between saving the hold and publishing `hold.requested`.** Holds saves, then publishes. If
  the process dies in between, the hold stays `PENDING`. The fix is a transactional outbox (save the
  event in the same transaction as the hold, and publish it from there).
- **No timeout for a `PENDING` hold** that Inventory never answers. A small sweeper that rejects holds
  `PENDING` for more than a few minutes would close this.
- **Inventory reports "no such reservation" and "wrong state" as plain `INTERNAL` errors**, so Holds
  cannot tell them from an outage and answers `UNAVAILABLE` for all of them. Mapping them to `NOT_FOUND`
  and `FAILED_PRECONDITION` in Inventory would let Holds give a better answer.
- **No lock-out for repeated wrong pickup codes.** A code has about a billion values, so guessing is not
  practical yet, but staff-facing access needs an attempt limit when the gateway exposes `CollectHold`.

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> postgres + migrations -> RabbitMQ + Inventory client -> app -> gRPC server and event consumer
internal/domain/             Hold, status machine, pickup codes, errors (no outside imports)
internal/ports/              HoldRepository, Inventory and Events interfaces
internal/app/                the use cases: create, get, list, reserve, reject, cancel, collect, expire; saga.go: HandleStockReserved and HandleStockUnavailable
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/clients/   Inventory gRPC client (ReleaseReservation, CommitReservation)
internal/adapters/rabbitmq/  publisher for hold.requested and hold.confirmed; consumer for stock.reserved and stock.unavailable
internal/adapters/postgres/  HoldRepository on holds_db
internal/adapters/memory/    in-memory HoldRepository (tests)
internal/adapters/repotest/  contract tests every HoldRepository must pass
migrations/                  SQL, applied automatically at startup
test/integration/            end-to-end saga test (build tag `e2e`)
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `HOLDS_GRPC_PORT` | `50065` | gRPC listen port |
| `HOLDS_DATABASE_URL` | required | Postgres connection URL for `holds_db` |
| `HOLDS_MAX_ACTIVE_PER_USER` | `3` | most `PENDING` + `ACTIVE` holds per patient (at least 1) |
| `HOLDS_HOLD_DURATION` | `2h` | how long an `ACTIVE` hold lasts (Go duration, e.g. `90m`) |
| `RABBITMQ_URL` | required | RabbitMQ connection URL (the saga's events) |
| `HOLDS_UPSTREAM` | `traefik:50051` | where calls to Inventory go. Traefik routes them by `:authority` (`inventory`). |
| `HOLDS_CALL_TIMEOUT` | `3s` | time limit for one attempt of an Inventory call (up to 3 attempts on `UNAVAILABLE`) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Run

```bash
make up    # whole stack; holds is reached through Traefik
grpcurl -plaintext -authority holds \
  -d '{"user_id":"u-1","pharmacy_id":"ph-1","product_id":"pr-1","quantity":2}' \
  localhost:50051 swiftmed.holds.v1.HoldsService/CreateHold
```

## Test

```bash
go test -cover ./...

# Repository contract tests against Postgres (use a throwaway database; it empties the holds table):
HOLDS_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/holds_test?sslmode=disable \
  go test -tags integration ./internal/adapters/postgres/

# Mutation testing (SCRUM-113), settings in .gremlins.yaml; or `make mutation` from the repo root:
GOWORK=off gremlins unleash ./internal/domain
GOWORK=off gremlins unleash ./internal/app
```

Mutation scores (Gremlins v0.5.0, every mutator): domain 95.2% efficacy (20 killed, 1 equivalent
mutant lived, 13 in tagless `switch` cases that Go's coverage profile cannot see), app 98.3% (57 killed,
1 equivalent lived, 1 constant not coverable). Details:
[docs/test-plan.md §7.3](../../docs/test-plan.md).

### End-to-end saga test

`test/integration/saga_e2e_test.go` runs the saga against the whole Docker Compose stack and checks the stock
numbers in Inventory after every step: reserve and collect (wrong code refused, right code works, collecting
twice fails, stock committed), not enough stock (`REJECTED`), cancel (stock released), cancel right after
create (stock never leaked, even when the reservation arrives late), and expiry (stock released).

```bash
# from the repo root: start the stack. A short hold time lets the expiry scenario run (it is skipped otherwise).
HOLDS_HOLD_DURATION=5s docker compose -f deploy/docker-compose.yml --env-file .env up -d --build

# from services/holds: the test calls Holds and Inventory through Traefik on localhost:50051
go test -tags e2e -count=1 -v ./test/integration/
```

Without Traefik, point the test at the services directly with `E2E_HOLDS_ADDR` and `E2E_INVENTORY_ADDR`.
