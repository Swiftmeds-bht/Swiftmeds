# Pricing service

Each pharmacy's price for a product. Owner: Pema. Supporting service.

This is also the **reference template**: copy its layout when starting a new service
(see [docs/new-service.md](../../docs/new-service.md)).

## Contract

[`proto/pricing/v1/pricing.proto`](../../proto/pricing/v1/pricing.proto)

| RPC | What it does |
|---|---|
| `SetPrices` | Sets one or more prices for a pharmacy. All or nothing: one invalid price rejects the whole request. |
| `BatchGetPrices` | Returns prices for (pharmacy, product) pairs. Pairs with no price are skipped. |

Money is `int64` chhertum (100 chhertum = Nu. 1). A price must be greater than 0.

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> adapters -> app -> gRPC server
internal/domain/             Price entity and validation rules (no outside imports)
internal/ports/              PriceRepository interface
internal/app/                SetPrices, BatchGetPrices use cases
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/memory/    in-memory PriceRepository (temporary)
internal/adapters/postgres/  Postgres PriceRepository (to do)
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PRICING_GRPC_PORT` | `50063` | gRPC listen port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Run

```bash
# Locally
go run ./cmd/server

# In Docker (build from the repo root)
docker build -f services/pricing/Dockerfile -t swiftmed/pricing .
docker run --rm -p 50063:50063 swiftmed/pricing
```

Try it with [grpcurl](https://github.com/fullstorydev/grpcurl) (reflection is on):

```bash
grpcurl -plaintext localhost:50063 list
grpcurl -plaintext -d '{"pharmacy_id":"ph-1","updates":[{"product_id":"pr-1","price_chhertum":4500}]}' \
  localhost:50063 swiftmed.pricing.v1.PricingService/SetPrices
grpcurl -plaintext localhost:50063 grpc.health.v1.Health/Check
```

## Test

```bash
go test -cover ./...
```

## To do

- Postgres adapter and migrations (replace `adapters/memory` in `main.go`)
