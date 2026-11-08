# Expiry service

Releases holds whose time has run out. Expiry knows **when** each confirmed hold ends; the Holds
service owns **what** happens to it. A scheduled sweep finds the holds that are due and asks
Holds to expire each one.

## Contract

[`proto/expiry/v1/expiry.proto`](../../proto/expiry/v1/expiry.proto)

| RPC | What it does |
|---|---|
| `RunSweep` | Expires every hold whose deadline is at or before `as_of` (now when `as_of` is left out) and returns the IDs it expired. Safe to repeat. |
| `GetHoldDuration` | How long a new hold lasts (`EXPIRY_HOLD_DURATION`, same for every pharmacy and product for now). |

Over HTTP the sweep is `POST /api/v1/internal/expiry/sweep` on the gateway
(see [`docs/api/openapi.yaml`](../../docs/api/openapi.yaml)); the Cloudflare hold-expiry worker
calls it on a schedule with the shared worker secret.

## How it works

1. **Deadlines come in.** Holds publishes `hold.confirmed` when a hold becomes ACTIVE, with its
   `expires_at`. Expiry's consumer (queue `expiry.hold-confirmed`, deduplicated, retried, then
   dead-lettered) stores one row per hold in `hold_deadlines`. A hold that is already stored keeps
   its first deadline, so a redelivered event changes nothing.
2. **The sweep.** `RunSweep(as_of)` reads the pending deadlines with `expires_at <= as_of`, oldest
   first, in pages of `EXPIRY_SWEEP_BATCH_SIZE`. For each one it calls `Holds.ExpireHold` and then
   settles the deadline with an outcome:

   | Holds answers | Deadline outcome | Listed in `expired_hold_ids` |
   |---|---|---|
   | hold is now `EXPIRED` | `EXPIRED` | yes |
   | hold was already `COLLECTED`, `CANCELLED` or `REJECTED` | `ALREADY_FINAL` | no |
   | hold not found | `NOT_FOUND` | no |
   | "not yet expired" (the two clocks differ by a moment) | stays pending | no, tried again next sweep |
   | unavailable / timeout | stays pending, the sweep stops | no |

3. **Idempotent.** A deadline is settled once (`UPDATE ... WHERE completed_at IS NULL`; only the call
   that changes the row counts). Running the sweep twice in a row calls `ExpireHold` once per hold
   and reports each hold once. Two sweeps at the same moment (overlapping schedules) also report each
   hold once, and `Holds.ExpireHold` is a no-op on a hold that is no longer ACTIVE.
4. **Boundary.** A hold that expires exactly at `as_of` is expired (`expires_at <= as_of`); one a
   nanosecond later is not. See `internal/domain/deadline_test.go` and `internal/app/sweep_test.go`.
5. **Partial progress.** If Holds goes down or the call runs out of time mid-sweep, the holds
   already expired stay expired and are returned; the rest stay pending for the next sweep. The
   gateway allows a sweep a few seconds, so a large backlog drains over several runs.

## Not in this service yet

- Holds does not publish `hold.confirmed` yet (the reservation saga, SCRUM-61). Until it does, no
  deadlines are stored and a sweep finds nothing. Everything else is in place, and the consumer
  is tested with built events.
- Releasing the reserved stock. `Holds.ExpireHold` will tell Inventory when SCRUM-61 wires that in;
  Expiry only needs the hold to become `EXPIRED`.
- Deleting old settled rows from `hold_deadlines`.

## Layout

```
cmd/server/main.go           wiring only: config -> postgres + migrations -> RabbitMQ -> Holds client -> app -> gRPC + consumer
internal/domain/             Deadline (and the "due exactly now" rule), outcomes, errors (no outside imports)
internal/ports/              DeadlineRepository, HoldExpirer interfaces
internal/app/                RecordDeadline, RunSweep, HoldDuration
internal/adapters/grpc/      ExpiryService handler: proto <-> app, errors -> status codes
internal/adapters/postgres/  DeadlineRepository on expiry_db
internal/adapters/memory/    in-memory DeadlineRepository (tests)
internal/adapters/repotest/  contract tests every DeadlineRepository must pass
internal/adapters/clients/   Holds gRPC client (ExpireHold)
internal/adapters/rabbitmq/  hold.confirmed consumer
migrations/                  SQL, applied automatically at startup
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `EXPIRY_GRPC_PORT` | `50066` | gRPC listen port |
| `EXPIRY_DATABASE_URL` | required | Postgres connection URL for `expiry_db` |
| `RABBITMQ_URL` | required | RabbitMQ connection URL |
| `EXPIRY_HOLDS_UPSTREAM` | `traefik:50051` | where to reach Holds (Traefik routes by authority `holds`) |
| `EXPIRY_HOLDS_CALL_TIMEOUT` | `2s` | limit for each `Holds.ExpireHold` attempt (one retry on `UNAVAILABLE`) |
| `EXPIRY_HOLD_DURATION` | `2h` | what `GetHoldDuration` answers; keep equal to `HOLDS_HOLD_DURATION` |
| `EXPIRY_SWEEP_BATCH_SIZE` | `50` | due deadlines read per page |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Test

```bash
go test -cover ./...

# Repository contract tests against Postgres (use a throwaway database; it empties hold_deadlines):
EXPIRY_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/expiry_test?sslmode=disable \
  go test -tags integration ./internal/adapters/postgres/
```
