# Search service

The patient's question, "which nearby pharmacy has this medicine?". Owner: Pema.
**Core service** (coverage gate: 80% on domain + app). Owns no data: it
orchestrates other services, all reached through Traefik.

## Contract

[`proto/search/v1/search.proto`](../../proto/search/v1/search.proto)

| RPC | What it does |
|---|---|
| `SearchMedicine` | Products matching the query, each with in-stock nearby pharmacies, best first. If none of them is in stock nearby, same-molecule `substitutes` that are |
| `Autocomplete` | Product names for a prefix, e.g. "Napa 500mg (Paracetamol)" |

| Input | Rule (else `INVALID_ARGUMENT`) |
|---|---|
| `query` / `prefix` | 2–100 characters after trimming |
| `location` | Required; latitude -90..90, longitude -180..180 |
| `radius_m` | 0 → 5000, otherwise 500..50000 |
| `max_offers` | 0 → 10, otherwise 1..50 (per product) |
| `limit` (autocomplete) | 0 → 8, otherwise 1..20 |

## How a search runs

```
          ┌─ Catalog.ResolveName ──┐   (at the same time)
query ────┤                        ├──► Inventory.BatchGetStock (in stock only)
          └─ Geo.FindNearby ───────┘                 │
                                                     ▼
       Freshness · Pricing · Trust · Pharmacy Registry   (at the same time)
                                                     │
                                                     ▼
                                     Ranking.RankOffers ──► results
                       (nothing in stock?) Substitution ──► substitutes
```

| Service | Needed? | If it fails or is too slow |
|---|---|---|
| Catalog, Geo, Inventory | **Required** | Search returns `UNAVAILABLE` (`DEADLINE_EXCEEDED` if out of time) |
| Freshness | Optional | Freshness 0 for every offer |
| Pricing | Optional | Price 0 ("not listed") |
| Trust | Optional | Neutral trust 0.5 (also used for a pharmacy Trust has no score for) |
| Pharmacy Registry | Optional | No name, address or map location |
| Ranking | Optional | Nearest first |
| Substitution | Optional | No substitutes |

When optional services did not answer, the response header
**`x-search-degraded`** lists them (e.g. `freshness,ranking`), so the gateway
can show "some details are unavailable" without a contract change.

**Time budget:** each downstream call has its own timeout (`SEARCH_CALL_TIMEOUT`,
800 ms) and a whole search is capped at `SEARCH_REQUEST_TIMEOUT` (2 s, the
proposal's target). Every client also uses `pkg/resilience` (circuit breaker,
retry with one extra attempt, bulkhead), one per downstream service.

Limits: 5 products, 100 nearby pharmacies, 5 substitutes per search.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `SEARCH_GRPC_PORT` | `50060` | gRPC listen port |
| `SEARCH_UPSTREAM` | `traefik:50051` | Where the downstream services are reached |
| `SEARCH_CALL_TIMEOUT` | `800ms` | Limit per downstream call |
| `SEARCH_REQUEST_TIMEOUT` | `2s` | Limit per search |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Test

```bash
go test -cover ./...
```

- `internal/app`: orchestration with fakes for all nine services: happy path,
  every optional service failing or timing out, required services failing,
  concurrency, the 2 s budget, substitutes.
- `internal/adapters/grpc`: the real handler, app and clients over in-memory
  gRPC against `internal/adapters/upstreamtest` (a fake of all nine services).
