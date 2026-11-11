# Architecture decision records

Each ADR records one architectural decision: the problem, what we chose, what it
costs us, and what else we looked at. Diagrams of the result are in
[docs/architecture](../architecture/README.md).

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-grpc-inside-rest-at-the-gateway.md) | gRPC between services, REST at the gateway | Accepted |
| [0002](0002-choreographed-saga.md) | Choreographed saga for reservations, not orchestration | Accepted |
| [0003](0003-database-per-service.md) | A database per service | Accepted |
| [0004](0004-traefik-for-discovery-and-load-balancing.md) | Traefik for service discovery and load balancing | Accepted |
| [0005](0005-docker-compose-main-stack-kubernetes-demo.md) | Docker Compose as the main stack, Kubernetes as a demo | Accepted (Kubernetes part planned, SCRUM-108) |

## Writing a new ADR

1. Copy the outline below into `NNNN-short-title.md`, using the next free
   four-digit number. Numbers are never reused.
2. Keep it short: one decision per ADR, and point to the code that shows it.
3. Add it to the table above in the same PR.
4. Never rewrite an accepted ADR to change the decision. Write a new one, and set
   the old one's status to `Superseded by NNNN`.

Status is one of `Proposed`, `Accepted`, `Deprecated` or `Superseded by NNNN`.

```markdown
# NNNN. Title

- **Status:** Proposed
- **Date:** YYYY-MM-DD
- **Deciders:** names

## Context

## Decision

## Consequences

## Alternatives considered
```
