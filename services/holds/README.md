# Holds service

Short reservations of stock for a patient. A patient holds up to 5 packs of a medicine at one
pharmacy, collects them with a **pickup code**, and the hold ends in `COLLECTED`, `EXPIRED`,
`CANCELLED` or `REJECTED`.

## Contract

[`proto/holds/v1/holds.proto`](../../proto/holds/v1/holds.proto)

| RPC | What it does | Errors |
|---|---|---|
| `CreateHold` | Stores a `PENDING` hold (quantity 1 to 5). | `INVALID_ARGUMENT`, `RESOURCE_EXHAUSTED` when the patient already has 3 open holds |
| `GetHold` | Returns one hold. The patient's pickup code is included while the hold is `ACTIVE`. | `NOT_FOUND` |
| `ListHolds` | Holds of a patient (`user_id`) **or** of a pharmacy (`pharmacy_id`), newest first, optional status filter, paged. Pickup codes are included only in the patient's list. | `INVALID_ARGUMENT` |
| `CancelHold` | The patient who made the hold cancels it. | `PERMISSION_DENIED` (not their hold), `FAILED_PRECONDITION` (already finished) |
| `CollectHold` | Pharmacy staff hand over the medicine. Needs `hold_id`, `pharmacy_id` and the patient's `pickup_code`. | `PERMISSION_DENIED` (wrong code or pharmacy), `FAILED_PRECONDITION` (already collected, expired, cancelled ...), `ABORTED` (lost a race with another change) |
| `ExpireHold` | Called by Expiry once `expires_at` has passed. Does nothing unless the hold is `ACTIVE`. | `FAILED_PRECONDITION` if called before `expires_at` |

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

## Not in this service yet

- `PENDING -> ACTIVE` (stock reserved) and `PENDING -> REJECTED` (stock unavailable) are the use cases
  `ReserveHold` and `RejectHold` in `internal/app`. They are **not RPCs**: the reservation saga
  (SCRUM-61) will call them when Inventory answers. Until then a hold created over gRPC stays `PENDING`.
- Publishing events and calling Inventory to release or commit stock (SCRUM-61).
- Locking out repeated wrong codes. A code has about a billion values, so guessing is not practical
  yet, but staff-facing access will need an attempt limit when the gateway exposes `CollectHold`.

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> postgres + migrations -> app -> gRPC server
internal/domain/             Hold, status machine, pickup codes, errors (no outside imports)
internal/ports/              HoldRepository interface
internal/app/                the use cases: create, get, list, reserve, reject, cancel, collect, expire
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/postgres/  HoldRepository on holds_db
internal/adapters/memory/    in-memory HoldRepository (tests)
internal/adapters/repotest/  contract tests every HoldRepository must pass
migrations/                  SQL, applied automatically at startup
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `HOLDS_GRPC_PORT` | `50065` | gRPC listen port |
| `HOLDS_DATABASE_URL` | required | Postgres connection URL for `holds_db` |
| `HOLDS_MAX_ACTIVE_PER_USER` | `3` | most `PENDING` + `ACTIVE` holds per patient (at least 1) |
| `HOLDS_HOLD_DURATION` | `2h` | how long an `ACTIVE` hold lasts (Go duration, e.g. `90m`) |
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
```
