# 0001. gRPC between services, REST at the gateway

- **Status:** Accepted
- **Date:** 2026-11-11 (recorded; the contracts in `proto/` were frozen after Sprint 1)
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

SwiftMed has 20 services owned by four people. Search alone calls up to nine
others for one request, so the calls between services must be fast, typed, and
hard to break by accident when someone changes a service they do not own. The
clients, on the other hand, are a Next.js app and two Cloudflare Workers, which
speak HTTP and JSON and should not need generated gRPC code or HTTP/2 trailers.

## Decision

- Services talk to each other only over **gRPC**, with contracts in
  `proto/<service>/v1/*.proto` and generated Go code in `proto/gen/go`
  (`make proto`). Events use the same protobuf messages
  (`proto/events/v1/events.proto`).
- Clients talk only to one **REST gateway** (`services/gateway`), described in
  `docs/api/openapi.yaml`. Each route there names the gRPC method behind it
  (`x-grpc`). The gateway translates JSON to gRPC and gRPC status codes to HTTP
  status codes, and it is the only service published to the outside.
- The gateway authenticates every non-public route itself: it reads the Bearer
  token and calls `AuthService/ValidateToken`, then checks the role for pharmacy
  and admin routes (`services/gateway/internal/httpx/auth.go`). Services behind it
  receive the user ID in the request message and do not see tokens.
- Gateway calls get a per-attempt timeout (800 ms by default) and one retry
  (`pkg/resilience`, used in `services/gateway/internal/grpcx/factory.go`).
  Only read-only RPCs are retried; every RPC's decision is listed in
  `pkg/resilience/methods.go`, and a test fails if a new RPC has none.
- `.proto` files are frozen after Sprint 1; any change needs the architecture
  lead's approval (README, "Branch rules").

## Consequences

- Breaking a contract is a compile error in every caller, not a runtime surprise.
- The gateway is the single place for auth, CORS, request IDs, logging and error
  mapping; services stay small and do not repeat it.
- Every new client-facing feature needs work in two places: the gRPC method and a
  gateway route, plus the OpenAPI entry.
- Streaming to the browser needs a translation step. Back-in-stock alerts (SCRUM-71,
  in progress) will turn the gRPC server stream `WatchlistService/StreamAlerts` into
  Server-Sent Events at `GET /api/v1/watchlist/alerts`; the gateway already runs
  without an HTTP write timeout for that reason (`services/gateway/cmd/server/main.go`).
- Debugging needs gRPC tools (`grpcurl`) or the gateway; you cannot curl a service.

## Alternatives considered

- **REST and JSON everywhere.** Simpler to debug, but no shared typed contract
  across 20 services and four owners, and slower serialisation on Search's fan-out.
- **gRPC to the browser (gRPC-Web or Connect).** Removes the gateway's translation,
  but needs a proxy or a different server library, generated clients in the web
  app, and does not suit the Workers' simple `fetch` calls.
- **A GraphQL gateway.** Flexible for the client, but a third schema to keep in
  step with the protos, for two clients whose needs are known.
