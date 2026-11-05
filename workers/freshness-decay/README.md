# Freshness decay Worker

A Cloudflare Worker that runs every hour (`0 * * * *`, UTC). It calls the
gateway, and the gateway asks the Freshness service to recompute every stored
stock confidence. Stock that has not been confirmed recently drops from HIGH to
MEDIUM to LOW. Owner: see [CODEOWNERS](../../.github/CODEOWNERS). SCRUM-74.

```
Worker (cron) --POST + secret--> gateway --gRPC ApplyDecay--> Freshness service --> freshness_db
```

## The call

Defined in [`docs/api/openapi.yaml`](../../docs/api/openapi.yaml) as `runFreshnessDecay`.

```
POST {GATEWAY_URL}/api/v1/internal/freshness/decay
X-Worker-Secret: <WORKER_SHARED_SECRET>
Content-Type: application/json

{"asOf": "2026-11-05T07:00:00.000Z"}
```

`asOf` is the current time when the Worker runs. The secret has its own header,
so it is never mistaken for a user's `Authorization: Bearer` token.

| Gateway answer | Meaning |
|---|---|
| `200 {"updatedCount": N}` | N stock rows changed confidence (`0` is normal) |
| `400` | `asOf` is missing or not an RFC 3339 timestamp |
| `401` | Missing or wrong secret, or the gateway has no secret configured |
| `503` / `504` | Freshness service unreachable or too slow |

A failed call is logged and the run is marked failed. There is no retry: the
next hourly run does the same work, and the decay is safe to repeat.

## Settings

| Name | Where | Meaning |
|---|---|---|
| `GATEWAY_URL` | `.dev.vars` locally, a variable in the Cloudflare dashboard when deployed | Gateway base URL, no path |
| `WORKER_SHARED_SECRET` | `.dev.vars` locally, `wrangler secret put WORKER_SHARED_SECRET` when deployed | Must equal the gateway's `WORKER_SHARED_SECRET` (at least 16 characters) |

Never commit `.dev.vars`. Generate a secret with `openssl rand -hex 32`.

## Run and test

```bash
npm ci
npm run typecheck
npm test
```

Try the cron by hand. Start the stack (gateway, Freshness, Postgres, Traefik) with the same
`WORKER_SHARED_SECRET` in the root `.env`, copy `.dev.vars.example` to `.dev.vars`, then:

```bash
npm run dev    # wrangler dev --test-scheduled
# in another terminal:
curl "http://localhost:8787/__scheduled?cron=0+*+*+*+*"
```
