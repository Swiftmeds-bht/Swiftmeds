# Pricing service

Each pharmacy's price for a product, and price comparison across pharmacies.
Owner: Pema. Supporting service.

This is also the **reference template**: copy its layout when starting a new service
(see [docs/new-service.md](../../docs/new-service.md)).

## Contract

[`proto/pricing/v1/pricing.proto`](../../proto/pricing/v1/pricing.proto)

| RPC | What it does |
|---|---|
| `SetPrices` | Sets one or more prices for a pharmacy. All or nothing: one invalid price rejects the whole request. |
| `BatchGetPrices` | Returns prices for (pharmacy, product) pairs as a **comparison**: grouped by product, cheapest first, equal prices ordered by pharmacy ID. Pairs with no price are left out. |

To compare one medicine across pharmacies, call `BatchGetPrices` with that product
at every pharmacy of interest; the first result is the cheapest.

Money is `int64` chhertum (100 chhertum = Nu. 1). A price must be greater than 0
(checked in the domain and by a database `CHECK`).

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> postgres + migrations -> app -> gRPC server
internal/domain/             Price entity, validation, comparison order (no outside imports)
internal/ports/              PriceRepository interface
internal/app/                SetPrices, BatchGetPrices use cases
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/postgres/  PriceRepository on pricing_db
internal/adapters/memory/    in-memory PriceRepository (tests)
internal/adapters/repotest/  contract tests every PriceRepository must pass
migrations/                  SQL, applied automatically at startup
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PRICING_GRPC_PORT` | `50063` | gRPC listen port |
| `PRICING_DATABASE_URL` | required | Postgres connection URL for `pricing_db` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Run

```bash
make up    # whole stack; pricing is reached through Traefik
grpcurl -plaintext -authority pricing \
  -d '{"pharmacy_id":"ph-1","updates":[{"product_id":"pr-1","price_chhertum":4500}]}' \
  localhost:50051 swiftmed.pricing.v1.PricingService/SetPrices
```

## Test

```bash
go test -cover ./...

# Repository contract tests against Postgres (use a throwaway database):
PRICING_TEST_DATABASE_URL='postgres://swiftmed:change-me@localhost:5432/pricing_test?sslmode=disable' \
  go test -tags integration ./internal/adapters/postgres/
```
