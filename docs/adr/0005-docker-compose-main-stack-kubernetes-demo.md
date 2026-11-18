# 0005. Docker Compose for local development, Kubernetes for deployment

- **Status:** Accepted, revised 18 Nov 2026 (SCRUM-108). The first version (11 Nov)
  made Compose the main stack and Kubernetes a side demo; on 12 Nov the team made
  Kubernetes the deployment platform. Both parts are in place.
- **Date:** 2026-11-11, revised 2026-11-18
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

Every team member must be able to start the whole backend (the gateway, Traefik,
RabbitMQ, Postgres, the Go services and the web apps) on their own machine, with
one command, to develop and to test. The project is delivered in five short
sprints, so the setup cost of each tool matters.

The hosted SwiftMed that examiners, UAT users and the final demo use must also
show what an orchestrator adds that Compose does not: self-healing, scaling on
load, and rolling updates with rollback.

## Decision

- **Docker Compose is the local development stack.** `deploy/docker-compose.yml`
  defines Postgres, RabbitMQ, Traefik, all 20 services and the two web apps;
  `make up`, `make seed` and `make down` wrap it. Configuration comes from `.env`
  (copied from `.env.example`; never committed). The Cypress tests (`make e2e`)
  run against it.
- **Kubernetes (k3s) is the deployment.** `deploy/k8s/` holds Kustomize manifests,
  applied with `kubectl apply -k deploy/k8s`: one Deployment, Service and Ingress
  route per service, Postgres and RabbitMQ as StatefulSets with persistent
  volumes, a ConfigMap for settings and a Secret made from a local file, gRPC
  readiness and liveness probes, resource requests and limits, and HPAs on Search
  and the gateway. It runs on the team laptop as k3s in Docker (k3d,
  `deploy/k8s/local`); the runbook is [docs/deployment.md](../deployment.md).
- **Traefik routes the same way in both.** In Compose it finds the services
  through its Docker provider ([ADR 0004](0004-traefik-for-discovery-and-load-balancing.md));
  in Kubernetes the same Traefik configuration reads standard Ingress objects. So
  every service still calls `traefik:50051` with the callee's name as `:authority`,
  and no service code or setting differs between the two. Traefik sends each gRPC
  call to a pod directly, which is what makes extra Search pods useful: a plain
  Kubernetes Service would pin each HTTP/2 connection to one pod.
- Images are built from the same Dockerfiles for both (`scripts/k8s-images.sh`)
  and published to the GitHub Container Registry (`ghcr.io/swiftmeds-bht/<service>`).
- CI does not use either: its jobs start Postgres and RabbitMQ as GitHub Actions
  service containers, and the saga tests start their own with testcontainers.

## Consequences

- One command still starts everything for development; new members need only
  Docker and `make`. The deployment needs k3d and kubectl as well.
- Two deployment descriptions must be kept in step. A new service or setting goes
  into both `deploy/docker-compose.yml` and `deploy/k8s/` (`config.env`, and
  `secrets.env.example` for secrets). The database init script and the demo
  fixtures are shared, not copied: the `seed` image carries them for Kubernetes.
- One Postgres server and one RabbitMQ broker remain single points of failure in
  both. Kubernetes restarts them and keeps their data on volumes, but they are not
  replicated: acceptable for the course deployment, not for production.
- A laptop host is only up while the laptop is; the public URL (SCRUM-75) goes
  through a Cloudflare Tunnel from it. Moving to a cloud VM later means installing
  k3s there and applying the same manifests.
- If the cluster ever fails before a demo, Compose remains a working fallback
  (risk R9).

## Alternatives considered

- **Kubernetes for development too.** One description instead of two, but a
  cluster on every laptop and slower feedback during sprints.
- **Kubernetes Services instead of Traefik for gRPC.** Fewer parts, but gRPC
  connections would not be spread over replicas, so the Search HPA would add pods
  that get no calls. A service mesh would fix that at much more cost.
- **Kompose output as is.** Kompose turns Compose labels into nothing useful for
  Traefik and gives no probes, limits or per-service secrets. Its idea was kept (a
  generator from the Compose file made the first drafts), the output was not.
- **Docker Swarm.** Adds rolling updates and replicas to Compose files, but is less
  used and teaches less than Kubernetes.
