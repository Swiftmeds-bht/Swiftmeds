# Freshness service

How much to trust a stock figure, given how long ago it was last confirmed.
Owner: Sherab. Core service.

## Contract

[`proto/freshness/v1/freshness.proto`](../../proto/freshness/v1/freshness.proto)

| RPC | What it does |
|---|---|
| `RecordConfirmation` | Stores when a stock row was confirmed, and its confidence right now. An older confirmation never replaces a newer one. Called by Inventory. |
| `GetFreshness` | Batch lookup for search results. Rows that were never confirmed are left out. |
| `ApplyDecay` | Recomputes every stored confidence as of a given time (now, if empty) and returns how many rows changed. Safe to run twice. Called by the freshness-decay Worker. |

## How confidence works

The age of the last confirmation decides the level. The level is sent to other services as a score.

| Age of last confirmation | Confidence | Score |
|---|---|---|
| under `FRESHNESS_HIGH_WITHIN` (6h) | HIGH | 1.0 |
| under `FRESHNESS_MEDIUM_WITHIN` (24h) | MEDIUM | 0.6 |
| anything older | LOW | 0.2 |

An age exactly on a threshold belongs to the lower level (exactly 6h is MEDIUM).

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> database -> app -> gRPC server
internal/domain/             Confidence, Thresholds, Confirmation, Rating (no outside imports)
internal/ports/              RatingRepository interface
internal/app/                RecordConfirmation, GetConfidence, BatchGetConfidence, Decay
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/postgres/  Postgres RatingRepository, and Migrate
migrations/                  SQL files, embedded in the binary and run at start
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `FRESHNESS_GRPC_PORT` | `50069` | gRPC listen port |
| `FRESHNESS_DATABASE_URL` | required | Postgres connection string for `freshness_db` |
| `FRESHNESS_HIGH_WITHIN` | `6h` | Younger than this is HIGH |
| `FRESHNESS_MEDIUM_WITHIN` | `24h` | Younger than this is MEDIUM, older is LOW. Must be longer than `FRESHNESS_HIGH_WITHIN`. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Database

`freshness_db`, one table: `stock_confirmation`, one row per pharmacy and product.
The service runs every `migrations/*.up.sql` file when it starts, so each file
must be safe to run more than once (`CREATE TABLE IF NOT EXISTS`). The
`*.down.sql` files are only for running by hand.

## Run

```bash
# Locally (needs a Postgres with freshness_db)
FRESHNESS_DATABASE_URL="postgres://swiftmed:change-me@localhost:5432/freshness_db?sslmode=disable" \
  go run ./cmd/server

# In Docker (build from the repo root)
docker build -f services/freshness/Dockerfile -t swiftmed/freshness .
```

Try it with [grpcurl](https://github.com/fullstorydev/grpcurl) (reflection is on):

```bash
grpcurl -plaintext localhost:50069 list
grpcurl -plaintext -d '{"key":{"pharmacy_id":"ph-1","product_id":"pr-1"},"confirmed_at":"2026-10-25T09:00:00Z"}' \
  localhost:50069 swiftmed.freshness.v1.FreshnessService/RecordConfirmation
grpcurl -plaintext -d '{"keys":[{"pharmacy_id":"ph-1","product_id":"pr-1"}]}' \
  localhost:50069 swiftmed.freshness.v1.FreshnessService/GetFreshness
grpcurl -plaintext -d '{}' localhost:50069 swiftmed.freshness.v1.FreshnessService/ApplyDecay
```

## Test

```bash
go test -cover ./...
```

The Postgres adapter tests need a database and are skipped without one. To run
them, point `FRESHNESS_TEST_DATABASE_URL` at any Postgres. The tests work in a
temporary schema and delete it afterwards, so they never touch real data:

```bash
FRESHNESS_TEST_DATABASE_URL="postgres://swiftmed:change-me@localhost:5432/freshness_db?sslmode=disable" \
  go test -cover ./...
```
