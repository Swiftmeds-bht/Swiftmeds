# 0003. A database per service

- **Status:** Accepted
- **Date:** 2026-11-11 (recorded after the fact)
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

Twenty services are built in parallel by four people. If services shared tables,
one person's schema change could break another person's service, and the services
could not be deployed, scaled or tested on their own.

At the same time the whole stack runs on each developer's machine with Docker
Compose ([ADR 0005](0005-docker-compose-main-stack-kubernetes-demo.md)), so
seventeen separate Postgres servers would cost a lot of memory and start-up time.

## Decision

- Each service that keeps data has **its own logical database**, `<name>_db`, on
  one shared Postgres 16 server in Compose (`deploy/postgres/init/01-databases.sql`).
  Each service receives only its own `<NAME>_DATABASE_URL` (`.env.example`).
- A service never reads or writes another service's database. It asks the owner
  over gRPC ([ADR 0001](0001-grpc-inside-rest-at-the-gateway.md)) or listens to its
  events ([ADR 0002](0002-choreographed-saga.md)).
- Each service owns its schema in `services/<name>/migrations` (embedded
  `*.up.sql` files) and applies it at startup with `pkg/migrate.Up`, which records
  versions in `schema_migrations` and takes a Postgres advisory lock so replicas
  starting together do not race. (Freshness applies its embedded migrations with
  its own helper.)
- Event consumers keep their deduplication table, `swiftmed_processed_events`, in
  their own database too (`pkg/messaging/deduper.go`).
- Services with no state of their own have no database: gateway, search, ranking
  and (in Compose) substitution.

## Consequences

- Schemas change independently; a migration in `holds_db` cannot break Inventory.
- Cross-service data must be fetched or copied through APIs and events. For
  example, Watchlist does not join against pharmacy locations; it asks Geo, and
  Search composes results from nine services at request time.
- There are no cross-service transactions, which is why reservations need a saga
  ([ADR 0002](0002-choreographed-saga.md)).
- One Postgres server is a shared failure point and shared capacity in Compose.
  The separation is logical, so moving a database to its own server later only
  changes that service's URL.
- All services use the same Postgres user in the local `.env`; separate users per
  database would make the isolation enforced rather than by convention.

## Alternatives considered

- **One shared database (or shared schema).** Easiest for queries across domains,
  but couples every service to every schema change and defeats independent
  deployment.
- **A Postgres server per service.** Strongest isolation, but seventeen extra
  containers on a developer machine for no gain at our scale.
- **Different stores per service** (for example Redis for Watchlist alerts).
  Possible later per service; Postgres alone keeps operations and testing
  (testcontainers) uniform for now.
