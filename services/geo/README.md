# Geo service

Where verified pharmacies are, and which ones are near a point. Owner: Pema.
Search calls `FindNearbyPharmacies`; Pharmacy Registry keeps the locations in sync.

## Contract

[`proto/geo/v1/geo.proto`](../../proto/geo/v1/geo.proto)

| RPC | What it does |
|---|---|
| `FindNearbyPharmacies` | Pharmacies within `radius_m` of `location`, **nearest first** (ties by pharmacy ID), at most `limit`. Distances are whole metres. |
| `UpsertPharmacyLocation` | Adds or moves a pharmacy (called when it is verified or moves) |
| `RemovePharmacyLocation` | Removes a pharmacy (e.g. suspended). Removing an unknown pharmacy succeeds |

| Input | Rule | Error |
|---|---|---|
| `location` | latitude -90..90, longitude -180..180 | `INVALID_ARGUMENT` |
| `radius_m` | 1..50000. A pharmacy exactly on the radius counts as inside. Search applies its own default before calling Geo | `INVALID_ARGUMENT` |
| `limit` | 0..100; 0 means 20 | `INVALID_ARGUMENT` |

The contract has `limit`, not a `PageRequest`: results are a short nearest-first
list, so pagination was not needed.

## How it works

1. **SQL pre-filter:** a latitude/longitude bounding box around the point, using
   the `(latitude, longitude)` index. The box is slightly too big on purpose.
2. **Exact check in Go:** Haversine great-circle distance (mean Earth radius
   6,371,008.8 m) removes the corners, then sort and limit.

No PostGIS needed. Accuracy: Thimphu→Phuentsholing is within 1% of the known
73.4 km straight-line distance (tested).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `GEO_GRPC_PORT` | `50062` | gRPC listen port |
| `GEO_DATABASE_URL` | required | Postgres connection URL for `geo_db` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

Migrations in `migrations/` are applied automatically at startup.

## Run and test

```bash
make up
grpcurl -plaintext -authority geo \
  -d '{"pharmacy_id":"ph-1","location":{"latitude":27.4728,"longitude":89.6390}}' \
  localhost:50051 swiftmed.geo.v1.GeoService/UpsertPharmacyLocation
grpcurl -plaintext -authority geo \
  -d '{"location":{"latitude":27.4728,"longitude":89.6390},"radius_m":5000}' \
  localhost:50051 swiftmed.geo.v1.GeoService/FindNearbyPharmacies

go test -cover ./...      # includes a test on deploy/seed/pharmacies.csv
GEO_TEST_DATABASE_URL='postgres://swiftmed:change-me@localhost:5432/geo_test?sslmode=disable' \
  go test -tags integration ./internal/adapters/postgres/
```
