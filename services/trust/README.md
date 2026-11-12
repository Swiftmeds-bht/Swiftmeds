# Trust service

Each pharmacy's accuracy record: when patients collect a medicine, did the
pharmacy really have it? Trust turns every `feedback.submitted` event into a
0..1 score, and Search passes that score to Ranking as the `trust` factor.
Owner: see [CODEOWNERS](../../.github/CODEOWNERS). SCRUM-73.

## Contract

[`proto/trust/v1/trust.proto`](../../proto/trust/v1/trust.proto)

| RPC | What it does | Errors |
|---|---|---|
| `GetTrust` | One pharmacy's score. A pharmacy without feedback gets the neutral `0.5`, `feedback_count` 0 and no `updated_at`. | `INVALID_ARGUMENT` (no `pharmacy_id`) |
| `BatchGetTrust` | One score per requested ID, in request order (Search calls this once per search). Unknown pharmacies are neutral. An empty list returns an empty list. | `INVALID_ARGUMENT` (a blank ID, more than 1000 IDs) |

`TrustScore` also carries `feedback_count`, `in_stock_count` (plain counts, not
weighted) and `updated_at` (when the newest feedback was given).

## The score

```
score = (prior·0.5 + Σ wᵢ·xᵢ) / (prior + Σ wᵢ)
```

- `xᵢ` is 1 when the patient said "was in stock" and 0 when they said "not in stock".
- `wᵢ = 0.5^(age / half-life)`: recent feedback counts more. With the default
  30-day half-life, an answer from today counts 1, from a month ago 0.5, from
  two months ago 0.25.
- `prior` is a number of imaginary neutral answers every pharmacy starts with
  (default 3).

What follows from it:

| Situation | Score |
|---|---|
| New pharmacy, no feedback | **0.5** (neutral, same as Search's fallback when Trust is down) |
| One "not in stock" | 0.375 |
| One "in stock" | 0.625 |
| 1000 "not in stock" | 0.0015, never below **0** |
| 1000 "in stock" | 0.9985, never above **1** (the max Ranking accepts) |
| Feedback stops coming | drifts back towards 0.5 |

With `TRUST_PRIOR_WEIGHT=0` the score is the plain weighted share of
"in stock" answers, and reaches exactly 0 or 1. The `rating` in the event is
about the pickup experience, not stock accuracy, so it does not count.

The score is not stored: it depends on when it is asked for. Trust stores, per
pharmacy, the two weighted sums (taken at the time of the newest feedback) and
the counts. Adding feedback is O(1), and feedback that arrives out of order is
weighted by its own age, so the order of events does not change the result.

## Events

Consumes `feedback.submitted` on queue `trust.feedback-submitted` (dead letters
go to `trust.feedback-submitted.dlq`). `pkg/messaging` deduplicates on the event
ID, retries three times and then dead-letters.

On top of that, every `feedback_id` is recorded in the same transaction that
updates the pharmacy's row. The same feedback therefore never counts twice, even
when Feedback publishes it again under a new event ID (see the Feedback README),
or when the service stops between updating the row and marking the event
processed. The pharmacy's row is locked while it is updated, so concurrent
feedback for one pharmacy is never lost.

## Configuration

| Variable | Default | |
|---|---|---|
| `TRUST_GRPC_PORT` | `50068` | |
| `TRUST_DATABASE_URL` | required | `trust_db` |
| `RABBITMQ_URL` | required | |
| `TRUST_HALF_LIFE` | `720h` | Feedback this old counts half. Must be positive. |
| `TRUST_PRIOR_WEIGHT` | `3` | Neutral answers a new pharmacy starts with. `0` or more. |
| `LOG_LEVEL` | `info` | |

In Docker Compose, Traefik routes `:authority` `trust` here; Search already
dials that name.

## Tests

```bash
cd services/trust
GOWORK=off go test ./...            # unit tests, in-memory repository
TRUST_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/trust_test?sslmode=disable \
  GOWORK=off go test -tags integration ./internal/adapters/postgres/   # same contract against Postgres
```

The boundary tests for the score (exactly 0 and exactly 1 without a prior,
never past either with one, neutral for a new pharmacy, saturating counts) are
in `internal/domain/trust_test.go`.
