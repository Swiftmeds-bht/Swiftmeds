# Watchlist service

"Tell me when this medicine is back in stock near me." Owner: Pema.

A patient subscribes to a product and an area (a point and a radius). When
Inventory publishes `stock.increased` for that product, Watchlist checks which
waiting subscribers have the pharmacy inside their radius, records an alert for
each of them, and marks their subscription notified so it does not fire again.

## API (`proto/watchlist/v1/watchlist.proto`)

| RPC | What it does |
|---|---|
| `Subscribe` | Creates a subscription. Subscribing again to the same product while still waiting returns the existing one. `radius_m` 0 means 5 km; the maximum is 50 km. |
| `Unsubscribe` | Removes one of the user's subscriptions and its alerts. Someone else's ID answers `NOT_FOUND`. |
| `ListSubscriptions` | The user's subscriptions, newest first. `notified_at` is set once the alert fired. |
| `StreamAlerts` | Server stream of the user's alerts (SCRUM-71). First sends every alert not yet delivered, oldest first, then each new one as it happens, until the client goes away. See below. |

## Alert stream (SCRUM-71)

`StreamAlerts(user_id)` stays open while the patient has the app open (the
gateway turns it into server-sent events at `GET /api/v1/watchlist/alerts`).

- **Delivered at least once, then not again.** An alert is marked delivered
  (`alerts.delivered_at`) only after `Send` succeeded. If the patient is offline
  when stock comes back, the alert waits and is sent on the next connection. If
  the connection breaks between `Send` and the mark, the alert can arrive twice;
  the patient app shows one alert per subscription.
- **Push and poll.** A saved alert wakes that user's open streams on the same
  replica at once (`internal/app/stream.go`, `hub`). Every stream also checks the
  database every 5 s (`DefaultAlertPoll`), so alerts saved by another replica
  are found too.
- **No leaks.** When the client cancels, the stream's context ends, the loop
  returns and the stream leaves the hub. The tests check the hub is empty after
  disconnect, with `-race`.

## Events

Consumes `stock.increased` on queue `watchlist.stock-increased` (dead letters go
to `watchlist.stock-increased.dlq`). Each event is processed once (deduplicated
on its event ID), retried three times on failure, then dead-lettered.

An event can only alert a subscription once: the alert and `notified_at` are
written in one transaction that requires `notified_at IS NULL`. A retry after a
partial failure therefore alerts only the subscribers not done yet.

## Matching by distance

`stock.increased` carries only the pharmacy ID. Watchlist asks Geo
`FindNearbyPharmacies(subscriber's point, radius)` and checks whether the
pharmacy is in the answer. Geo returns at most the nearest 100 pharmacies, so a
pharmacy with more than 100 others closer to the subscriber is treated as out of
range. That is far beyond Bhutan's pharmacy counts.

## Configuration

| Variable | Default | |
|---|---|---|
| `WATCHLIST_GRPC_PORT` | `50064` | |
| `WATCHLIST_DATABASE_URL` | required | `watchlist_db` |
| `RABBITMQ_URL` | required | |
| `WATCHLIST_UPSTREAM` | `traefik:50051` | Where Geo is reached; Traefik routes on `:authority` `geo`. |
| `WATCHLIST_GEO_TIMEOUT` | `2s` | Per Geo call; up to 3 attempts. |
| `LOG_LEVEL` | `info` | |

## Tests

```bash
go test ./...                       # unit tests, in-memory repository
WATCHLIST_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/watchlist_test?sslmode=disable \
  go test -tags integration ./internal/adapters/postgres/   # same contract against Postgres
```
