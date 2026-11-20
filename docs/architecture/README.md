# Architecture

How SwiftMed is put together: which parts exist, how they talk to each other, and
why. Everything here is checked against the code on `develop`; when the code
changes, update these pages in the same PR.

| Page | What it shows |
|---|---|
| [System context and containers](system-context.md) | The people and outside systems around SwiftMed, then every container: the gateway, Traefik, the 20 services, RabbitMQ, Postgres, the Workers and the web apps |
| [Reservation saga](reservation-saga.md) | Sequence diagrams of the choreographed hold saga: the happy path, no stock, cancel, expiry, and the late-reservation compensation |
| [Architecture decision records](../adr/README.md) | Why the main choices were made (ADR 0001 to 0005) |
| [Security risk register](../security-risk-register.md) | SCRUM-99 privacy and security risk, controls, and sprint-review status log |

## Summary

- **20 Go services** (Go 1.26, one module each in `go.work`). Each has a hexagonal
  layout, `internal/{domain,ports,app,adapters}`; see
  [docs/new-service.md](../new-service.md).
- **REST at the edge, gRPC inside.** Browsers and Workers call the REST gateway
  (`services/gateway`, port 8080). The gateway and the services call each other over
  gRPC through Traefik's `grpc` entrypoint (`:50051`), which routes each call by its
  `:authority` (the service name). See [ADR 0001](../adr/0001-grpc-inside-rest-at-the-gateway.md)
  and [ADR 0004](../adr/0004-traefik-for-discovery-and-load-balancing.md).
- **Auth.** The gateway reads `Authorization: Bearer <token>` and asks the Auth service
  to validate it (`ValidateToken`, HS256 JWTs). Search, nearby and login routes are
  public; `/api/v1/pharmacy*` needs pharmacy staff and `/api/v1/admin*` needs admin
  (`services/gateway/internal/httpx/auth.go`).
- **Events.** RabbitMQ carries protobuf events on one topic exchange,
  `swiftmed.events` ([proto/events/v1/events.proto](../../proto/events/v1/events.proto)).
  `pkg/messaging` gives every consumer its own durable queue, deduplication on the
  event ID (table `swiftmed_processed_events` in the consumer's database), three
  attempts, then a dead-letter queue `<queue>.dlq` behind the `swiftmed.events.dlx`
  exchange.
- **Data.** One Postgres server in Compose with one database per service
  (`deploy/postgres/init/01-databases.sql`). A service touches only its own
  database and migrates it at startup with `pkg/migrate` (Freshness uses its own
  migration helper). See [ADR 0003](../adr/0003-database-per-service.md).
- **Reservations** run as a choreographed saga between Holds and Inventory, with
  compensation through `ReleaseReservation`. See [ADR 0002](../adr/0002-choreographed-saga.md)
  and [reservation-saga.md](reservation-saga.md).
- **Scheduled jobs** are two Cloudflare Workers that call internal gateway routes
  with `X-Worker-Secret`: `workers/hold-expiry` every minute and
  `workers/freshness-decay` every hour.
- **Clients.** One Next.js web app (`web/swiftmed`, SCRUM-139): patients at `/`,
  pharmacy staff at `/pharmacy-portal`, one sign-in whose session carries the role.
  It calls the gateway from the server only (server actions and route handlers), so
  the access token stays in an httpOnly cookie.
- **Deployment.** Docker Compose (`deploy/docker-compose.yml`) is the main stack;
  a Kubernetes demo of the core path is planned (SCRUM-108). See
  [ADR 0005](../adr/0005-docker-compose-main-stack-kubernetes-demo.md).

## Where to read more

| Topic | Source |
|---|---|
| REST API | [docs/api/openapi.yaml](../api/openapi.yaml) |
| gRPC and event contracts | [proto/](../../proto/) |
| Compose stack, Traefik, Postgres init | [deploy/](../../deploy/) |
| Saga integration tests | [tests/integration/saga](../../tests/integration/saga/) |
| Test plan | [docs/test-plan.md](../test-plan.md) |
| Per-service details | `services/<name>/README.md` and `workers/<name>/README.md` |
