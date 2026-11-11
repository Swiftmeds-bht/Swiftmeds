# Hold expiry Worker

A Cloudflare Worker that runs every minute (`* * * * *`, UTC). It calls the
gateway, and the gateway asks the Expiry service to expire every hold whose
deadline has passed. Each expired hold releases its reserved stock. Owner: see
[CODEOWNERS](../../.github/CODEOWNERS). SCRUM-63.

```
Worker (cron) --POST + secret--> gateway --gRPC RunSweep--> Expiry service --gRPC ExpireHold--> Holds service
```

## The call

Defined in [`docs/api/openapi.yaml`](../../docs/api/openapi.yaml) as `runHoldExpirySweep`.

```
POST {GATEWAY_URL}/api/v1/internal/expiry/sweep
X-Worker-Secret: <WORKER_SHARED_SECRET>
Content-Type: application/json

{"asOf": "2026-11-05T07:00:00.000Z"}
```

`asOf` is the current time when the Worker runs. The secret has its own header,
so it is never mistaken for a user's `Authorization: Bearer` token.

| Gateway answer | Meaning |
|---|---|
| `200 {"expiredHoldIds": [...]}` | The holds expired by this run (an empty list is normal) |
| `400` | `asOf` is missing or not an RFC 3339 timestamp |
| `401` | Missing or wrong secret, or the gateway has no secret configured |
| `503` / `504` | Expiry service unreachable or too slow |

A failed call is logged and the run is marked failed. There is no retry: the
next run, a minute later, does the same work, and a sweep is safe to repeat.
Each run logs one JSON line with `expiredCount` and up to 20 hold IDs.

## Settings

| Name | Where | Meaning |
|---|---|---|
| `GATEWAY_URL` | `.dev.vars` locally, a variable in the Cloudflare dashboard when deployed | Gateway base URL, no path |
| `WORKER_SHARED_SECRET` | `.dev.vars` locally, `wrangler secret put WORKER_SHARED_SECRET` when deployed | Must equal the gateway's `WORKER_SHARED_SECRET` (at least 16 characters) |

Never commit `.dev.vars`. Generate a secret with `openssl rand -hex 32`. The
freshness-decay Worker uses the same secret and the same gateway setting.

## Run and test

```bash
npm ci
npm run typecheck
npm test
```

Try the cron by hand. Start the stack (gateway, Expiry, Holds, Postgres, Traefik) with the same
`WORKER_SHARED_SECRET` in the root `.env`, copy `.dev.vars.example` to `.dev.vars`, then:

```bash
npm run dev    # wrangler dev --test-scheduled
# in another terminal:
curl "http://localhost:8787/__scheduled?cron=*+*+*+*+*"
```

Deploy: `wrangler secret put WORKER_SHARED_SECRET`, set `GATEWAY_URL`, then `npm run deploy`.
