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
- **Next.js** powers the patient web app (`web/patient-app`). The pharmacy dashboard (`web/pharmacy-dashboard`) is not built yet (SCRUM-58, SCRUM-70).

Diagrams (system context, containers, events, the reservation saga) are in
[docs/architecture](docs/architecture/README.md), and the reasons behind the main
choices are in the [architecture decision records](docs/adr/README.md).

```
proto/      gRPC and event contracts (generated code goes in proto/gen/go)
pkg/        shared Go code: resilience, messaging, grpcx, auth, logger, config
services/   one folder per microservice
workers/    Cloudflare Workers (TypeScript)
web/        patient-app (pharmacy-dashboard is a placeholder for now)
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
make up                 # build and start every service in deploy/docker-compose.yml, Postgres, RabbitMQ and Traefik
make seed               # load repeatable demo medicines, pharmacies, stock and synthetic prices
make down               # stop everything
```

`make seed` starts the services whose migrations own the demo data, waits for
those migrations, and imports the pharmacy, Geo, Inventory, and Pricing
fixtures. Catalog medicines are loaded by the Catalog migration and are not
imported again. Pharmacy CSV phone, address, and dzongkhag values are fixture
metadata because the current Pharmacy Registry schema does not store them.
Pricing uses deterministic synthetic values from `deploy/seed/prices.demo.csv`;
these are not real prices. The import is transactional and safe to repeat
without truncating tables or removing active reservations.

The gateway listens on `http://localhost:8080`; its health check is available at
`http://localhost:8080/healthz`. The Traefik dashboard is available at
`http://localhost:8081/dashboard/`, and the RabbitMQ management UI is available at
`http://localhost:15672`. Run `make help` to list all targets.

Gateway API routes use `Authorization: Bearer <access-token>`. The public routes
are `GET /api/v1/search`, `GET /api/v1/pharmacies/nearby` and
`GET /api/v1/medicines/{id}/alternatives`. The holds, watchlist and feedback
routes validate the token through Auth; pharmacy and admin routes additionally
require their respective roles. Missing or invalid credentials return `401`,
insufficient roles return `403`, and an unavailable Auth service returns `503`.
The two `/api/v1/internal/...` routes called by the Cloudflare Workers check the
`X-Worker-Secret` header (`WORKER_SHARED_SECRET` in `.env`) instead of a token.

The gateway has **no login or register routes yet** (SCRUM-115).
`POST /api/v1/auth/login` is described in `docs/api/openapi.yaml`, but the gateway
answers `404` until SCRUM-115 is merged, so you cannot get an access token
through the gateway today.

On Windows PowerShell, if `make` is not installed, use Docker Compose directly
from the repository root. Pass `--env-file .env` as `make` does: Compose
otherwise looks for `.env` in `deploy/` (the folder of the Compose file) and
fails with `required variable ... is missing a value`.

```powershell
docker compose -f deploy/docker-compose.yml --env-file .env up -d --build
docker compose -f deploy/docker-compose.yml --env-file .env ps
docker compose -f deploy/docker-compose.yml --env-file .env down
```

Search and Inventory are discovered by Traefik through Docker labels and use
round-robin load balancing. To run two replicas of each service:

```powershell
docker compose -f deploy/docker-compose.yml --env-file .env up -d --scale search=2 --scale inventory=2
docker compose -f deploy/docker-compose.yml --env-file .env logs -f search inventory traefik
```

The active backends are visible in the Traefik dashboard at
`http://localhost:8081/dashboard/`. Stopping one replica removes it from
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

### Tests

```bash
make test               # unit tests in every Go module
make lint               # golangci-lint in every Go module
make coverage           # core services must keep >= 80% on domain + app (CI enforces this)
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
