# How to start a new service

Every SwiftMed service has the same shape, so anyone can find their way around any of them.
**`services/pricing` is the reference.** Copy it, rename it, and replace the pricing logic with yours.

Below, `<name>` is the folder name (e.g. `stock-sync`) and `<Name>` the Go/proto name (e.g. `StockSync`).

## 1. Copy the template

```bash
cp -r services/pricing/{cmd,Dockerfile,README.md} services/<name>/
mkdir -p services/<name>/internal/{domain,ports,app,adapters/grpc}
```

Copy the pricing `internal/` files as a starting point if it helps, but rename the
types and delete the pricing rules. Keep the folder names.

## 2. Create the Go module

`services/<name>/go.mod`:

```
module github.com/Swiftmeds-bht/Swiftmeds/services/<name>

go 1.25.0

require (
	github.com/Swiftmeds-bht/Swiftmeds/pkg v0.0.0
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go v0.0.0
)

replace (
	github.com/Swiftmeds-bht/Swiftmeds/pkg => ../../pkg
	github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go => ../../proto/gen/go
)
```

Then:

```bash
cd services/<name> && GOWORK=off go mod tidy && cd ../..
```

and add `./services/<name>` to the `use` block in [`go.work`](../go.work).

> **Why both `replace` and `go.work`?** `go.work` lets your editor and `make test` see all
> modules at once. `replace` is what `go mod tidy` and the Docker build use, because both
> run with the workspace turned off.

## 3. Fill in the layers (inside out)

| Layer | Folder | Rules |
|---|---|---|
| Domain | `internal/domain/` | Entities and business rules. **Standard library only.** Return sentinel errors (`var ErrX = errors.New(...)`). Write these first, test-first. |
| Ports | `internal/ports/` | Interfaces for what the app needs from outside: repositories, event publishers, other services. |
| App | `internal/app/` | Use cases. Depends on `domain` and `ports` only. Take the clock as `func() time.Time` so tests can fix the time. Test with hand-written fakes of the ports. |
| Inbound adapter | `internal/adapters/grpc/` | Embed `Unimplemented<Name>ServiceServer`, translate proto ↔ app types, map domain errors to gRPC codes (`InvalidArgument`, `NotFound`, ...). No business logic. |
| Outbound adapters | `internal/adapters/{postgres,rabbitmq,clients}/` | Implement the ports. Add `var _ ports.X = (*Y)(nil)` so the compiler checks it. |

The dependency arrows only point inward: `adapters → app → domain`, never the other way.

## 4. Wire it in `cmd/server/main.go`

Change only the constants and the wiring block:

```go
const (
	serviceName = "<name>"
	defaultPort = 500xx            // from .env.example
	portEnv     = "<NAME>_GRPC_PORT"
)
```

```go
repo := postgres.NewRepository(db)          // outbound adapters
svc := app.NewService(repo, time.Now)       // use cases
srv := grpcx.NewServer(log)                 // shared server: logging, recovery, health
<name>v1.Register<Name>ServiceServer(srv, grpcadapter.NewHandler(svc))
return srv.Serve(ctx, fmt.Sprintf(":%d", port))
```

Read settings with `pkg/config`. Call `cfg.Required(...)` for anything without a safe
default (database URL, secrets) and check `cfg.Err()` once. The service then reports
every missing variable at startup, not just the first.

## 5. Dockerfile

In the copied Dockerfile, replace `pricing` with `<name>` (3 `COPY`/`RUN` lines) and
set `EXPOSE` to your port. Build from the **repo root**:

```bash
docker build -f services/<name>/Dockerfile -t swiftmed/<name> .
```

## 6. Check before opening the PR

- [ ] `make test` passes, and the domain and app layers have tests
- [ ] `go vet ./...` and `gofmt -l .` are clean
- [ ] The image builds, and `docker ps` shows the container as `healthy`
- [ ] `grpcurl -plaintext localhost:<port> list` shows your service
- [ ] `README.md` updated: contract table, config table, how to run

## What `pkg/` gives you

| Package | Use it for |
|---|---|
| `pkg/config` | Reading env vars: `String`, `Required`, `Int`, `Bool`, `Duration`, then `Err()` |
| `pkg/logger` | `logger.New("<name>", level)`: JSON logs with a `service` field |
| `pkg/grpcx` | `NewServer` (logging + panic recovery + health + reflection), `Serve` (graceful shutdown on SIGTERM), `CheckHealth` (for the `healthcheck` sub-command) |

Coming later: `pkg/messaging` (RabbitMQ), `pkg/resilience` (circuit breaker, retry,
timeout, bulkhead) and `pkg/auth` (JWT).
