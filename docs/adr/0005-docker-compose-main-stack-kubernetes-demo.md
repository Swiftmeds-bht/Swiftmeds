# 0005. Docker Compose as the main stack, Kubernetes as a demo

- **Status:** Accepted. The Compose part is in place; the Kubernetes demo is planned (SCRUM-108, not started).
- **Date:** 2026-11-11
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

Every team member must be able to start the whole backend (the gateway, Traefik,
RabbitMQ, Postgres and the Go services) on their own machine, with one command,
to develop and to demo. The project is delivered in five short sprints, so the setup cost of each tool
matters.

We still want to show what an orchestrator adds that Compose does not:
self-healing, scaling on load, and rolling updates.

## Decision

- **Docker Compose is the main stack** for development, testing and the demo.
  `deploy/docker-compose.yml` defines Postgres, RabbitMQ, Traefik, the gateway and
  every service that has a Dockerfile; `make up` and `make down` wrap it, and the
  README shows the plain `docker compose` commands for Windows. Configuration comes
  from `.env` (copied from `.env.example`; never committed).
- Discovery and balancing in Compose come from Traefik's Docker provider
  ([ADR 0004](0004-traefik-for-discovery-and-load-balancing.md)), and replicas are
  shown with `docker compose up --scale search=2 --scale inventory=2`.
- CI does not use the Compose file: its jobs start Postgres and RabbitMQ as
  GitHub Actions service containers, and the saga tests start their own with
  testcontainers (`.github/workflows/ci.yml`, `tests/integration/saga`).
- The Cloudflare Workers run on Cloudflare (or `wrangler dev`) and the web apps
  run with `npm`; they are not in Compose and reach the stack through the gateway.
- **Kubernetes is a demo only**, added beside Compose without changing it
  (SCRUM-108): manifests in `deploy/k8s/` for a local minikube or kind cluster,
  covering the core path (gateway, search, inventory, holds, one Postgres and
  RabbitMQ), with probes, a ConfigMap and Secret, an HPA on Search, and a runbook
  showing self-healing, scaling and a rolling update.

## Consequences

- One command starts everything; new members need only Docker and `make`.
- Compose gives no self-healing beyond `restart: unless-stopped`, no autoscaling
  and no rolling updates; the Kubernetes demo exists to show those.
- One Postgres server and one RabbitMQ broker are single points of failure in
  Compose. That is acceptable for development and the demo, not for production.
- Two deployment descriptions must be kept in step for the core path once the
  Kubernetes demo exists; the rest of the services stay Compose-only.
- Traefik's Docker provider does not apply in Kubernetes, so the demo will use
  Kubernetes Services (and optionally an Ingress) instead, and the callers'
  `*_UPSTREAM` settings will point there.
- Until SCRUM-108 is done there is no Kubernetes deployment in the repository.

## Alternatives considered

- **Kubernetes as the main stack.** Closer to production, but a cluster on every
  laptop, more YAML than code for 20 services, and slower feedback during sprints.
- **Run services directly with `go run`.** Fast for one service, but every
  developer would have to install and wire Postgres, RabbitMQ and Traefik by hand.
- **Docker Swarm.** Adds rolling updates and replicas to Compose files, but is less
  used and teaches less than Kubernetes for the demo.
