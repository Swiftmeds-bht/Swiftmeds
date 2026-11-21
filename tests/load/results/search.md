### search (2026-11-18T18:44:44.910Z)

Target: `http://localhost:9080`

- Requests: 6976, failed: 0.00%
- Latency: p50 10 ms, p95 14 ms, max 121 ms
- Virtual users: 100 (hold 2m)
- Degraded answers (optional service down): 0.00%

| Metric | Threshold | Result |
|---|---|---|
| `http_req_duration{name:search}` | `p(95)<2000` | pass |
| `http_req_failed{name:search}` | `rate<0.01` | pass |
| `checks` | `rate>0.99` | pass |
