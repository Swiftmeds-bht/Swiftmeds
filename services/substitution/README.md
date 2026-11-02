# Substitution Service

The Substitution service finds alternative products for a medicine by querying
Catalog. It does not own a database.

## Matching rules

- Exclude the source product, controlled products, and products with a different
  molecule.
- Mark a candidate `SUBSTITUTE_MATCH_TYPE_EXACT` only when its molecule, normalized
  strength, and dosage form match the source product and neither product is
  prescription-only.
- If either product is prescription-only, or the strength or dosage form differs,
  mark the candidate `SUBSTITUTE_MATCH_TYPE_NEEDS_PHARMACIST_CHECK`.
- Return exact matches before candidates needing pharmacist review.

These results classify suggestions. They do not authorize dispensing or replace
a lawful prescription.

## API

The gRPC service implements `SubstitutionService.GetSubstitutes`. The response
includes the source molecule ID and each candidate's product ID, names, strength,
and match type.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `SUBSTITUTION_GRPC_PORT` | `50058` | gRPC listen port |
| `SUBSTITUTION_UPSTREAM` | `traefik:50051` | Address used to reach downstream services |
| `SUBSTITUTION_CALL_TIMEOUT` | `800ms` | Timeout for Catalog calls |
| `LOG_LEVEL` | `info` | Log level |

Catalog calls go through Traefik with the `catalog` authority.

## Run and verify

Create a local `.env` from `.env.example` if needed, then start the Compose
stack from the repository root with `make up`.

Run the service checks with:

```bash
cd services/substitution
go test ./...
go vet ./...