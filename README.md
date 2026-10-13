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

- **20 Go microservices**, each with its own Postgres database and a hexagonal (ports and adapters) layout.
- **REST in, gRPC out:** clients call the API gateway over REST, and services talk to each other over gRPC.
- **RabbitMQ** carries events. The reservation saga (Holds and Inventory) runs on choreographed events.
- **Traefik** handles service discovery and round-robin load balancing.
- **Cloudflare Workers** run scheduled jobs: expiring reservations and decaying stock freshness.
- **Next.js** powers the patient web app and the pharmacy dashboard.

```
proto/      gRPC and event contracts (generated code goes in proto/gen/go)
pkg/        shared Go code: resilience, messaging, grpcx, auth, logger, config
services/   one folder per microservice
workers/    Cloudflare Workers (TypeScript)
web/        patient-app and pharmacy-dashboard
deploy/     Docker Compose, Traefik, RabbitMQ, Postgres init
tests/      e2e (Cypress), load (k6), features (Gherkin)
docs/       SRS, ADRs, API spec, test plan
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
make up                 # start all services, Postgres, RabbitMQ and Traefik
make down               # stop everything
```

The gateway listens on `http://localhost:8080`. Run `make help` to list all targets.

### Tests

```bash
make test               # unit tests in every Go module
make lint               # golangci-lint in every Go module
```

### Adding a service module

Create `services/<name>/go.mod` with the module path `github.com/Swiftmeds-bht/Swiftmeds/services/<name>`, then add `./services/<name>` to the `use` block in `go.work`.

## Branch rules

| Branch | Purpose | Who merges |
|---|---|---|
| `main` | Demo-ready. Tagged at the end of each sprint (`v0.1.0` = Sprint 1). | Release PR from `develop` only |
| `develop` | Integration branch. Default target for all PRs. | Via reviewed PR |
| `feature/SM-<n>-short-name` | New feature for Jira ticket SM-n | Via PR into `develop` |
| `fix/SM-<n>-short-name` | Bug fix found during the sprint | Via PR into `develop` |
| `test/SM-<n>-short-name` | New or improved tests only | Via PR into `develop` |
| `docs/SM-<n>-short-name` | Documentation only (SRS, ADRs, test plan) | Via PR into `develop` |
| `chore/SM-<n>-short-name` | Tooling, config, CI, dependencies | Via PR into `develop` |
| `hotfix/SM-<n>-short-name` | Urgent fix to the demo build on `main` | Via PR into `main`, then merged back into `develop` |

1. Branch off `develop`. The only exception is `hotfix/`, which branches off `main`.
2. Open a PR into `develop`. CODEOWNERS requests the right reviewer automatically.
3. Merge needs **1 approval** and **green CI** (lint, unit and integration tests, coverage gate).
4. Never push directly to `main` or `develop`.
5. `.proto` files are frozen after Sprint 1. Any contract change needs the architecture lead's approval.

**Commit and PR titles** follow [Conventional Commits](https://www.conventionalcommits.org/) with the Jira key:

```
feat(inventory): reserve stock for a hold (SM-12)
fix(search): handle empty radius (SM-31)
chore(repo): add Makefile (SM-1)
```

Branch prefixes and commit types are separate. A `feature/SM-12-reserve-stock` branch still uses `feat(...)` in its commit and PR titles, because `feat` is the Conventional Commits type.
