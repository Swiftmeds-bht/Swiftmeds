# Pharmacy Registry service

Registers pharmacies and controls whether they are visible to patients. New
registrations start as `PENDING`; only `VERIFIED` pharmacies are returned when
the verified-only filter is used.

## Contract

[`proto/pharmacy-registry/v1/pharmacy_registry.proto`](../../proto/pharmacy-registry/v1/pharmacy_registry.proto)

| RPC | What it does |
|---|---|
| `RegisterPharmacy` | Creates a pending pharmacy with owner, licence, GeoPoint and weekly hours. |
| `GetPharmacy` | Returns a pharmacy by ID, including pending and rejected records. |
| `BatchGetPharmacies` | Returns known records for requested IDs. |
| `ListPharmacies` | Lists all statuses or verified-only records. |
| `SetVerificationStatus` | Admin transition to verified, rejected or suspended. |

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PHARMACY_REGISTRY_GRPC_PORT` | `50053` | gRPC listen port |
| `PHARMACY_REGISTRY_DATABASE_URL` | required | Postgres connection URL |
| `LOG_LEVEL` | `info` | Log level |

## Test

```bash
go test -cover ./...
PHARMACY_REGISTRY_TEST_DATABASE_URL='postgres://...' go test -tags integration ./internal/adapters/postgres/
```

Weekly hours are stored as day-of-week entries (`0` Sunday through `6` Saturday)
with `HH:MM` opening and closing times.
