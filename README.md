# SwiftMed

**Find which nearby pharmacy has your medicine before you leave home.**

In Bhutan, a patient looking for a medicine has to phone or visit pharmacies one at a time. SwiftMed shows which verified pharmacies nearby have a medicine in stock right now. Patients can reserve it for a short time and collect it with a pickup code. If nothing is available, SwiftMed suggests products with the same molecule and strength, or alerts the patient when stock returns.

Results are ranked only on distance, how recently the stock was confirmed, the pharmacy's accuracy record and price. There is no paid placement.

SwiftMed is the combined final project for **WEB303** (Microservices and Serverless Applications), **SWE302** (Software Testing and Quality Assurance) and **SWE303** (Software Project Management) at the College of Science and Technology, Royal University of Bhutan (2026).

## Team

| Member | Role | Services |
|---|---|---|
| Jigme Ngawang Chogyal | Scrum Master | Catalog, Inventory, Stock Sync, Substitution, Analytics |
| Pema Losel Maurer | Architecture lead (.proto contracts) | Search, Ranking, Geo, Pricing, Watchlist |
| Sherab Nima Rigzin | Product Owner | Holds, Expiry, Feedback, Trust, Freshness |
| Yeshey Zhennue | QA lead (test plan, CI gates) | Gateway, Auth, User Profile, Pharmacy Registry, Notification |

Tutors: Mr. Pema Namgay and Mr. Nima

## Architecture

- **20 Go microservices** planned, each with its own Postgres database (except Gateway, Search, Ranking and Substitution, which own no data) and a hexagonal (ports and adapters) layout. 18 are built and run in Docker Compose; `services/analytics` and `services/notification` are still empty scaffolds and are not in `go.work` or the Compose file yet.
- **REST in, gRPC out:** clients call the API gateway over REST, and services talk to each other over gRPC.
- **RabbitMQ** carries events. The reservation saga (Holds and Inventory) runs on choreographed events.
- **Traefik** handles service discovery and round-robin load balancing.
- **Cloudflare Workers** run scheduled jobs: expiring reservations and decaying stock freshness.
- **Next.js** powers the patient web app (`web/patient-app`) and the pharmacy dashboard (`web/pharmacy-dashboard`, SCRUM-58, SCRUM-70). `make up` runs both in Docker.

Diagrams (system context, containers, events, the reservation saga) are in
[docs/architecture](docs/architecture/README.md), and the reasons behind the main
choices are in the [architecture decision records](docs/adr/README.md).

```
proto/      gRPC and event contracts (generated code goes in proto/gen/go)
pkg/        shared Go code: resilience, messaging, grpcx, auth, logger, config
services/   one folder per microservice
workers/    Cloudflare Workers (TypeScript)
web/        patient-app and pharmacy-dashboard (Next.js, each with its own Dockerfile)
deploy/     Docker Compose, Traefik, RabbitMQ, Postgres init, demo seed data
scripts/    proto generation, coverage gate, integration and mutation test runners
tests/      integration (reservation saga), load (k6); e2e and features are placeholders
docs/       ADRs, architecture diagrams, API spec (docs/api/openapi.yaml), test plan
```

## How to run

### Prerequisites

- Go 1.25+
- Docker with Compose v2
- `make`
- `protoc` and the Go plugins:
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```
- `golangci-lint` (for `make lint`)
- Node.js 20+ (for `web/` and `workers/`)

### Start everything

```bash
cp .env.example .env    # then change the secrets
make proto              # generate gRPC code
make up                 # build and start all 20 services, Postgres, RabbitMQ and Traefik, wait until all are healthy, then seed demo data
make smoke              # search -> reserve -> collect through the gateway, to check the stack works
make seed               # load the demo data again (safe to repeat)
make down               # stop everything
```

`make up` waits until every container reports healthy (up to `UP_TIMEOUT`
seconds, 900 by default; a first build on a clean machine compiles all 20
images). It then runs the one-shot `seed` container (`deploy/seed/seed.sh`),
which imports the pharmacy, Geo, Inventory, and Pricing fixtures and creates
the demo accounts. Catalog medicines are loaded by the Catalog migration and
are not imported again. Pharmacy CSV phone, address, and dzongkhag values are fixture
metadata because the current Pharmacy Registry schema does not store them.
Pricing uses deterministic synthetic values from `deploy/seed/prices.demo.csv`;
these are not real prices. The import is transactional and safe to repeat
without truncating tables or removing active reservations.

Demo accounts (development only; the password for all of them is `swiftmeds-demo`):

| Account | Sign in as |
|---|---|
| Patient | `patient@demo.swiftmeds.test` |
| Pharmacist of each seeded pharmacy | its license number, for example `demo-tim-001@demo.swiftmeds.test` for Demo Pharmacy Thimphu 01 |

A pharmacist registered through the API is not linked to any pharmacy, so it
cannot collect holds; use a seeded pharmacist for that.

| What | Address |
|---|---|
| Patient app | `http://localhost:3000` (`PATIENT_APP_HTTP_PORT`) |
| API gateway | `http://localhost:8080`, health check at `/healthz` |

The default Compose stack publishes only the patient app and gateway. gRPC,
PostgreSQL, RabbitMQ, the pharmacy dashboard and the Traefik dashboard are not
published on the host. For local debugging, add `-f deploy/docker-compose.dev.yml`
to Compose commands; that override publishes the internal ports on `127.0.0.1`
only. For example:

```powershell
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml --env-file .env up -d
```

Set `JWT_SECRET` and `WORKER_SHARED_SECRET` in `.env` to independently generated
secrets of at least 32 bytes (for example, `openssl rand -hex 32`). Auth refuses
to start with a short or placeholder JWT secret, and the gateway refuses to start
with a short or placeholder worker secret. The worker secret must match the
value configured in both Workers. Copying `.env.example` alone will not start the
stack until both secrets are replaced.

Logout revokes the refresh token immediately. Existing access tokens are
stateless and remain usable until their configured `JWT_TTL` expires; the default
window is 15 minutes after logout.

If a port is already taken on your machine, change it in `.env` (for example
`PATIENT_APP_HTTP_PORT=3071`). Traefik serves the patient app on its own
entrypoint; both web apps call the gateway inside the Docker network
(`GATEWAY_URL=http://gateway:8080`), so the browser never calls the gateway
directly. Both show live data from the seeded database. The patient app only
uses its built-in demo data with `USE_DEMO_DATA=true`. Run `make help` to list
all targets.

Gateway API routes use `Authorization: Bearer <access-token>`. The public routes
are `GET /api/v1/search`, `GET /api/v1/search/autocomplete`,
`GET /api/v1/pharmacies/nearby`, `GET /api/v1/medicines/{id}/alternatives`, and
`POST /api/v1/auth/register`, `/login` and `/refresh`. The holds, watchlist and
feedback routes validate the token through Auth; pharmacy and admin routes
additionally require their respective roles. Missing or invalid credentials
return `401`, insufficient roles return `403`, and an unavailable Auth service
returns `503`. The two `/api/v1/internal/...` routes called by the Cloudflare
Workers check the `X-Worker-Secret` header (`WORKER_SHARED_SECRET` in `.env`)
instead of a token.

On Windows PowerShell, if `make` is not installed, use Docker Compose directly
from the repository root. Pass `--env-file .env` as `make` does: Compose
otherwise looks for `.env` in `deploy/` (the folder of the Compose file) and
fails with `required variable ... is missing a value`.

```powershell
# make up
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build --wait --wait-timeout 900
docker compose -f deploy/docker-compose.yml --env-file .env --profile seed run --rm seed
docker compose -f deploy/docker-compose.yml --env-file .env ps
# make smoke
powershell -ExecutionPolicy Bypass -File scripts/smoke-test.ps1
# make down
docker compose -f deploy/docker-compose.yml --env-file .env down
```

Search and Inventory are discovered by Traefik through Docker labels and use
round-robin load balancing. To run two replicas of each service:

```powershell
docker compose -f deploy/docker-compose.yml --env-file .env up -d --scale search=2 --scale inventory=2
docker compose -f deploy/docker-compose.yml --env-file .env logs -f search inventory traefik
```

For local debugging, start the stack with `deploy/docker-compose.dev.yml` to
see active backends in the Traefik dashboard at `http://localhost:8081/dashboard/`.
Stopping one replica removes it from
Traefik's backend list while the remaining replica continues serving requests.
`docker compose stop` takes service names (it would stop every `search`
replica), so stop one replica container by name. The Compose project is called
`deploy`, so the containers are `deploy-search-1`, `deploy-search-2` and so on:

```powershell
docker compose -f deploy/docker-compose.yml --env-file .env ps search   # list the replica names
docker stop deploy-search-2
```

Do not add `-v` to the `down` command unless you intentionally want to delete the
local PostgreSQL and RabbitMQ data volumes.

### Deploy on Kubernetes

Docker Compose is for development; the deployment runs on Kubernetes (k3s), on
the team laptop through k3d ([ADR 0005](docs/adr/0005-docker-compose-main-stack-kubernetes-demo.md)).
Manifests are in [`deploy/k8s/`](deploy/k8s): a shared `base` and two overlays.

```bash
kubectl apply -k deploy/k8s/overlays/prod   # the deployment: GHCR images v1.0.0, 2 replicas, sealed secret
deploy/k8s/overlays/local/up.sh             # a development cluster from scratch: images built here, seeded
```

The runbook, with secrets, releases, rollbacks, scaling, backup and restore, and
the measured self-healing, autoscaling and rolling-update demos, is
[docs/deployment.md](docs/deployment.md).

### Tests

```bash
make test               # race-enabled unit tests in every Go module
make lint               # golangci-lint in every Go module
make coverage           # every service must keep >= 65% on domain + app (CI enforces this)
make proto-check        # fail if generated protobuf code differs from proto/
make openapi-lint       # lint docs/api/openapi.yaml
./scripts/integration-tests.sh   # Postgres/RabbitMQ integration tests (see below)
```

`./scripts/integration-tests.sh` runs every `//go:build integration` package. Each
test reads its database or broker URL from its own variable (for example
`HOLDS_TEST_DATABASE_URL`, `RABBITMQ_URL`) and **skips** when it is not set. To
really run them, create the test databases and export the variables exactly as
the `integration` job in `.github/workflows/ci.yml` does.

The reservation saga tests start their own Postgres and RabbitMQ with
testcontainers (Docker must be running):

```bash
cd tests/integration
GOWORK=off go test -tags integration -race -count=1 -v ./...
```

Load tests use k6; see [tests/load/README.md](tests/load/README.md). The k6
results (SCRUM-76, SCRUM-77) have not been recorded yet. They will go in the
[Results table](tests/load/README.md#results) of that README and in the test
summary report.

CI (`.github/workflows/ci.yml`) runs five jobs on every PR: **Go checks** (vet, lint,
unit tests with `-race`, coverage gate), **Go integration tests**, **Saga integration
tests**, **Web app** (patient app lint, typecheck, tests, build) and **Workers**.

### Adding a service

Copy `services/pricing`, the reference template. Step-by-step instructions are in [docs/new-service.md](docs/new-service.md).

## Branch rules

| Branch | Purpose | Who merges |
|---|---|---|
| `main` | Demo-ready. Tagged at the end of each sprint (`v0.1.0` = Sprint 1). | Release PR from `develop` only |
| `develop` | Integration branch. Default target for all PRs. | Via reviewed PR |
| `feature/SCRUM-<n>-short-name` | New feature for Jira ticket SCRUM-n | Via PR into `develop` |
| `fix/SCRUM-<n>-short-name` | Bug fix found during the sprint | Via PR into `develop` |
| `test/SCRUM-<n>-short-name` | New or improved tests only | Via PR into `develop` |
| `docs/SCRUM-<n>-short-name` | Documentation only (SRS, ADRs, test plan) | Via PR into `develop` |
| `chore/SCRUM-<n>-short-name` | Tooling, config, CI, dependencies | Via PR into `develop` |
| `hotfix/SCRUM-<n>-short-name` | Urgent fix to the demo build on `main` | Via PR into `main`, then merged back into `develop` |

SM-1 to SM-8 in early history are pre-Jira labels. Mapping: SM-1→SCRUM-90, SM-2→SCRUM-10, SM-3→SCRUM-20, SM-4→SCRUM-19, SM-5→SCRUM-43, SM-6→SCRUM-31, SM-7→SCRUM-24, SM-8→SCRUM-26.

1. Branch off `develop`. The only exception is `hotfix/`, which branches off `main`.
2. Open a PR into `develop`. CODEOWNERS requests the right reviewer automatically.
3. Merge needs **1 approval** and **green CI** (lint, unit and integration tests, coverage gate).
4. Never push directly to `main` or `develop`.
5. `.proto` files are frozen after Sprint 1. Any contract change needs the architecture lead's approval.

**Commit and PR titles** follow [Conventional Commits](https://www.conventionalcommits.org/) with the Jira key:

```
feat(inventory): add atomic stock reservation (SCRUM-36)
fix(search): handle empty radius (SCRUM-49)
chore(repo): add Makefile (SCRUM-90)
```

Branch prefixes and commit types are separate. A `feature/SCRUM-36-reserve-stock` branch still uses `feat(...)` in its commit and PR titles, because `feat` is the Conventional Commits type.
