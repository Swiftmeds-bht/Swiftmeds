# Gateway

The gateway applies in-memory fixed-window limits to public account endpoints:

- `POST /api/v1/auth/login`: per client IP and normalized email.
- `POST /api/v1/auth/register`: per client IP.

By default each key can make 10 requests per minute. Set `AUTH_RATE_LIMIT` to
change the request count and `AUTH_RATE_WINDOW` to change the window using a Go
duration such as `30s` or `5m`. When a key reaches its limit, the gateway
returns HTTP `429`, a JSON `{"error": ...}` body, and a `Retry-After` header
with the number of seconds until that key can try again. The counters are
local to each gateway process and reset when the process restarts.

The gateway uses the last valid address in `X-Forwarded-For` (Traefik appends
the address it observed) and falls back to the connection address when that
header is absent.
