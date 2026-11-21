# 0004. Traefik for service discovery and load balancing

- **Status:** Accepted
- **Date:** 2026-11-11 (recorded after the fact)
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

The gateway and several services (Search, Stock Sync, Substitution, Holds, Expiry,
Feedback, Watchlist) call other services over gRPC
([ADR 0001](0001-grpc-inside-rest-at-the-gateway.md)). Callers need to find a
healthy instance of the service they want, and we want to show that a service
can run as several replicas (`--scale search=2`) and keep serving when one stops.
Every caller should not have to know every service's host and port, and we did not
want to run a separate registry such as Consul for a Compose stack.

gRPC complicates plain DNS round robin: a gRPC client keeps one long-lived HTTP/2
connection, so Docker's DNS would pin each caller to one replica.

## Decision

- Run **Traefik v3** (`deploy/traefik/traefik.yml`) with the Docker provider and
  `exposedByDefault: false`. A service opts in with labels in
  `deploy/docker-compose.yml`: a router ``Host(`<name>`)`` on the `grpc`
  entrypoint, its port, and `scheme=h2c` (plaintext HTTP/2).
- Every gRPC caller dials one address, `traefik:50051`, and sets the call's
  `:authority` to the target service name with `grpc.WithAuthority`
  (`services/gateway/internal/grpcx/factory.go`, and each caller's
  `cmd/server/main.go` with its `*_UPSTREAM` setting). Traefik routes on that
  authority and balances requests round robin across the containers it finds.
- The same Traefik also fronts the REST gateway on the `web` entrypoint (`:8080`)
  and serves its dashboard on `:8081` (local development only, no auth).

## Consequences

- Adding or scaling a service needs only Compose labels; callers change nothing.
  Stopping a replica removes it from Traefik's backend list (README, "Start
  everything").
- Balancing happens per request, not per connection, because the callers' HTTP/2
  connection ends at Traefik.
- Traefik is on every internal call: one extra hop, and a single point of failure
  in Compose. Callers bound each call with a timeout (for
  example `SEARCH_CALL_TIMEOUT`, and `pkg/resilience` in the gateway), so a missing
  Traefik shows as an error rather than a hang.
- Traffic between services is plaintext h2c inside the Docker network; TLS would
  have to be added at Traefik and in the clients for a real deployment.
- Traefik needs read access to the Docker socket.
- Traefik v3.6 or later is required with Docker Engine 29 (older versions used a
  Docker API version that Engine 29 rejects; see the comment in the Compose file).

## Alternatives considered

- **Docker DNS names only** (`search:50060`). No extra component, but no real
  load balancing for long-lived gRPC connections, and each caller needs every port.
- **Client-side load balancing** (gRPC `round_robin` over DNS, or xDS). No proxy
  hop, but every service needs the configuration, and it does not show replicas in
  a dashboard.
- **Consul or etcd as a registry.** A real registry with health checks, but another
  stateful system to run and wire into 20 services.
- **Nginx or Envoy.** Both proxy gRPC well, but need hand-written configuration for
  every service instead of discovering containers from labels.
