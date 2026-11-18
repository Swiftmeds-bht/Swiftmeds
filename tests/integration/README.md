# Integration tests

Tests that run several real services together. They need Docker.

## Reservation saga (`saga/`, SCRUM-56)

Checks the choreographed reservation saga between Holds and Inventory
(SCRUM-61, see `services/holds/README.md`) end to end:

| Scenario | What it proves |
|---|---|
| Happy path | `CreateHold` is `PENDING`, then `ACTIVE` with a pickup code and expiry. Inventory shows the units reserved. `hold.requested`, `stock.reserved` and `hold.confirmed` all went over RabbitMQ. |
| Stock unavailable | Asking for more than is in stock makes the hold `REJECTED` ("stock unavailable"), with no pickup code. Nothing is reserved and no `hold.confirmed` is sent. |
| Hold expires | `ExpireHold` before `expires_at` is refused and keeps the stock. After it, the hold is `EXPIRED` and the units go back to available. A second call changes nothing. |
| Hold cancelled | `CancelHold` on an active hold gives the units back. |
| Duplicate events | `hold.requested` and `stock.reserved` delivered again, both with the same event ID and with new ones, leave exactly one reservation and the same active hold. |

After every compensation (expire, cancel) the tests check that `quantity_reserved` is back to 0:
stock is never left reserved.

### How it works

`TestMain` starts **Postgres 16** and **RabbitMQ 3.13** with
[testcontainers](https://golang.testcontainers.org/) (the same images as Docker
Compose) and creates `holds_db` and `inventory_db`. It then builds the real
`services/holds` and `services/inventory` binaries (with `-race`) and runs them
against the containers. Holds reaches Inventory directly (`HOLDS_UPSTREAM`), not
through Traefik. Holds runs with `HOLDS_HOLD_DURATION=3s`, so the expiry test
waits only a few seconds.

The tests talk gRPC to both services, the way the gateway and the Expiry
service do. A private queue bound to `swiftmed.events` records every event, so
the tests can check what was sent and replay a message to test duplicates. Each
test uses its own pharmacy, product and patient IDs, so the tests share one
stack without seeing each other's stock.

If a test fails, both services' logs are printed.

### Run

```bash
cd tests/integration
go test -tags integration -race -count=1 ./...
```

The first run pulls `postgres:16-alpine`, `rabbitmq:3.13-management` and the
testcontainers clean-up container (Ryuk). On a slow connection, skip Ryuk; the
tests stop their own containers at the end:

```bash
TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration -race -count=1 ./...
```

CI runs this in the `Saga integration tests` job.

## Compose gateway suite (`compose/`, SCRUM-67)

The Compose suite runs the high-risk cross-service journeys through the
gateway on the full stack: seeded search, reservation and expiry, one-shot
watchlist restock alert, and feedback flowing into the trust score used by
search. It uses the deterministic fixtures in `deploy/seed`; the watchlist
case publishes the same protobuf `stock.increased` event that Inventory
publishes so the alert path is exercised without adding a test-only API.

Run it after starting and seeding Compose:

```bash
cd tests/integration
GATEWAY_URL=http://127.0.0.1:8080 \
RABBITMQ_URL=amqp://swiftmed:change-me@127.0.0.1:5672/ \
WORKER_SHARED_SECRET=change-me-worker-secret \
GOWORK=off go test -tags integration -count=1 -v ./compose
```

CI runs this package in the **Compose gateway integration suite** job, which
starts the full Compose stack, seeds it, runs the tests, prints logs on
failure, and removes the stack and volumes.

### Versions

`testcontainers-go` is pinned to **v0.44.0**, the newest release that supports
Go 1.25 (v0.45 needs Go 1.26). This module has its own `go.mod`, so its test
dependencies do not reach any service.
