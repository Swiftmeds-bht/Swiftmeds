# Feedback service

After a patient collects a hold, they say whether the medicine was really there. Feedback stores
that answer (one per hold) and publishes `feedback.submitted`, which Trust and Analytics consume.
Owner: see [CODEOWNERS](../../.github/CODEOWNERS). SCRUM-64.

## Contract

[`proto/feedback/v1/feedback.proto`](../../proto/feedback/v1/feedback.proto)

| RPC | What it does | Errors |
|---|---|---|
| `SubmitFeedback` | Stores one feedback for a **collected** hold and publishes `feedback.submitted`. Needs `hold_id` and `user_id`. `rating` is 0 (not given) or 1 to 5. `comment` is at most 500 characters. | `INVALID_ARGUMENT` (missing id, bad rating, long comment), `NOT_FOUND` (no such hold), `PERMISSION_DENIED` (not the patient's hold), `FAILED_PRECONDITION` (hold is not `COLLECTED`), `ALREADY_EXISTS` (the hold already has feedback), `UNAVAILABLE` (Holds or the event bus is down; nothing was saved, try again) |
| `ListPharmacyFeedback` | A pharmacy's feedback, newest first, paged (default 20, at most 100). | `INVALID_ARGUMENT` (no `pharmacy_id`, bad page token) |

Event: routing key `feedback.submitted` ([`events.proto`](../../proto/events/v1/events.proto)).
It carries the feedback, hold, pharmacy and product IDs, `was_in_stock` and `rating`, and a
`metadata.event_id` (a new UUID for every event) for consumers to deduplicate on. It has **no
patient ID and no comment**: minimal personal data (risk R8).

## Rules

- **Only for a collected hold.** Feedback asks Holds (`GetHold`) and accepts the hold only when its
  status is `COLLECTED`. Pending, active, expired, cancelled and rejected holds are refused.
- **Only the patient who made the hold.** The checks run in this order: complete request, rating and
  comment valid, hold exists, the hold is this patient's, the hold was collected. A stranger therefore
  never learns the state of someone else's hold.
- **One feedback per hold.** A unique index on `hold_id` enforces it, so two submissions at the same
  moment cannot both be stored. The pharmacy and product come from the hold, never from the caller.
- **The event and the feedback go together.** The event is sent inside the database transaction,
  just before the commit. If sending fails, the transaction is rolled back, nothing is saved, and the
  patient can submit again (`UNAVAILABLE`). One rare gap remains: an event that was sent but a commit
  that then failed. A retry would send a second event with a new `event_id` for the same hold. A
  transactional outbox would close it; it is not built yet.

## What is not covered

- **"Wrong price".** The ticket mentions it, but the frozen contract has only `was_in_stock`, a rating
  and a comment, and `FeedbackSubmitted` has no field for it. Adding one means changing `proto/`, which
  needs the architecture lead.
- **"Or a visit".** The contract has no visit, only holds, so feedback needs a collected hold.
- **Gateway routes.** The gateway does not expose Feedback yet.

## Config

| Variable | Default | Meaning |
|---|---|---|
| `FEEDBACK_GRPC_PORT` | `50067` | gRPC port |
| `FEEDBACK_DATABASE_URL` | required | `feedback_db` |
| `RABBITMQ_URL` | required | where `feedback.submitted` is published |
| `FEEDBACK_UPSTREAM` | `traefik:50051` | where Holds is reached (Traefik routes by `:authority`) |
| `FEEDBACK_HOLDS_TIMEOUT` | `2s` | limit per Holds call (retried up to 3 times) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Run and test

```bash
cd services/feedback
GOWORK=off go test ./...                                  # unit tests, no database needed
GOWORK=off go vet ./...

# Postgres repository tests, against a throwaway database:
FEEDBACK_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/feedback_test?sslmode=disable \
  GOWORK=off go test -tags integration ./internal/adapters/postgres/
```

With Docker: `docker compose -f deploy/docker-compose.yml --env-file .env up -d --build feedback`,
then `grpcurl -plaintext localhost:50067 list` (the port is not published by default; use the
override file or `docker exec`).

## Layout

```
cmd/server/main.go           wiring only: config -> logger -> postgres + migrations -> Holds client, RabbitMQ -> app -> gRPC server
internal/domain/             Feedback, the rating and comment rules, errors (standard library only)
internal/ports/              FeedbackRepository, HoldReader, EventPublisher
internal/app/                the use cases: submit, list
internal/adapters/grpc/      gRPC handler: proto <-> app, errors -> status codes
internal/adapters/postgres/  FeedbackRepository on feedback_db
internal/adapters/memory/    in-memory FeedbackRepository (tests)
internal/adapters/repotest/  contract tests every FeedbackRepository must pass
internal/adapters/clients/   Holds client
internal/adapters/rabbitmq/  publishes feedback.submitted
migrations/                  SQL, applied automatically at startup
```
