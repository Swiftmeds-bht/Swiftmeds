# Substitution Service

The Substitution service finds alternative products for a medicine by querying
Catalog. It does not own a database.

## Matching rules

- Exclude the source product, controlled products, and products with a different molecule.
- A candidate with the same normalized strength and dosage form is marked
  `SUBSTITUTE_MATCH_TYPE_EXACT`.
- A candidate with the same molecule but a different strength or dosage form is
  marked `SUBSTITUTE_MATCH_TYPE_NEEDS_PHARMACIST_CHECK`.
- Exact matches are returned first.

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

From the repository root, configure `.env` and start the Compose stack with
`make up`. To run the service checks locally:

```bash
cd services/substitution
go test ./...
go vet ./...