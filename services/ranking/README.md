# Ranking service

Orders pharmacy offers for a search. Owner: Pema. **Core service** (coverage gate:
80% on domain + app). Pure computation: no database, no outbound calls, no clock.
Only objective factors count; there is no paid placement.

## Contract

[`proto/ranking/v1/ranking.proto`](../../proto/ranking/v1/ranking.proto): `RankOffers(candidates, weights?)`.

## How the score works

```
score = w.distance·distance + w.freshness·freshness + w.trust·trust + w.price·price
```

Each component is 0..1, where 1 is best:

| Component | How it is computed |
|---|---|
| distance | Min-max across the candidates: nearest = 1, farthest = 0 |
| price | Min-max across the candidates: cheapest = 1, dearest = 0. An unknown price (0) counts as the **median** of the known prices, so not listing a price is neither rewarded nor punished |
| freshness, trust | Already 0..1 scores (from Freshness and Trust), used as given |

If every candidate has the same distance (or price), they all get 1 for it.

- **Out of stock is never ranked:** candidates with `quantity_available = 0` are dropped.
- **Ties** are broken by distance, then pharmacy ID, then product ID, so the result
  never depends on input order.
- **`breakdown`** holds each factor's *weighted* contribution; the four add up to `score`.

### Weights

Default: distance **0.40**, freshness **0.25**, trust **0.20**, price **0.15**.
Override per request (`weights`), or for the whole service with
`RANKING_WEIGHT_DISTANCE/FRESHNESS/TRUST/PRICE`. Weights must be non-negative and
sum to 1; otherwise `INVALID_ARGUMENT` (per request) or the service refuses to start.

### Example (default weights)

| Pharmacy | Distance | Freshness | Trust | Price | Score |
|---|---|---|---|---|---|
| A | 500 m → 1.0 | 0.5 | 0.5 | Nu. 6 → 0.0 | 0.40 + 0.125 + 0.10 + 0 = **0.625** |
| B | 1500 m → 0.0 | 1.0 | 1.0 | Nu. 4 → 1.0 | 0 + 0.25 + 0.20 + 0.15 = **0.600** |

A ranks first: being three times closer outweighs B's fresher stock and lower price.

## Validation

`INVALID_ARGUMENT` for: missing pharmacy or product ID, negative distance, price or
quantity, freshness/trust outside 0..1, invalid weights, more than 1000 candidates.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `RANKING_GRPC_PORT` | `50061` | gRPC listen port |
| `RANKING_WEIGHT_*` | see above | Default weights |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Test

```bash
go test -cover ./...
make coverage                      # core-service gate
gremlins unleash ./internal/domain # mutation testing (SCRUM-51)
```
