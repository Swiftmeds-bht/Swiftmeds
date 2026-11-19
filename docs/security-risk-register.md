# Security risk register

## SCRUM-100 — Demo infrastructure outage or data loss (R9)

Added in Sprint 1; this risk was not in the original proposal register.

| Field | Value |
|---|---|
| Category | Technical / infrastructure |
| Probability | Low |
| Impact | Medium |
| Strategy | Transfer |
| Response | Scheduled jobs run on Cloudflare Workers and the demo uses managed hosting, so the provider carries responsibility for uptime and backups. Local fallback: the full stack runs on a laptop with `make up`. |
| Warning sign | Demo environment or a Worker cron fails during testing (SCRUM-75). |
| Owner | QA lead (Zhennue) |

### Status log

Add a dated status comment at each sprint review.

- **7 Nov update:** SCRUM-108 (Kubernetes demo) moved to Sprint 6 (14–17 Nov).
  The fallback trigger is 16 Nov: if the cluster is not stable by then,
  Kubernetes goes to "future work" and the demo runs on Docker Compose only.
- **27 Oct update:** Added SCRUM-108 (Kubernetes demo, Sprint 5, 8 pts).
  Kubernetes is in the syllabus, so a small demo was added alongside Docker
  Compose rather than migrating to Kubernetes.
  **Response: Mitigate.** Compose stays the main development and demo stack.
  Kubernetes covers only the core path (gateway, search, inventory, holds,
  Postgres and RabbitMQ) on minikube/kind. If the cluster is not stable by
  12 Nov, it is dropped to "future work" in the report and the demo runs on
  Compose. The final demo does not depend on Kubernetes.

## SCRUM-99 — User data privacy and security

| Field | Value |
|---|---|
| Category | Security |
| Probability | Low |
| Impact | High |
| Strategy | Mitigate |
| Response | Require JWT authentication for protected routes (SCRUM-43, SCRUM-34); use encrypted connections; collect only the personal data needed; do not store prescriptions. |
| Warning sign | Failed security test |
| Owner | QA lead (Zhennue) |

### Controls and current coverage

- The gateway validates JWTs for protected routes and enforces staff and admin
  roles. The Auth service signs HS256 tokens, uses bcrypt for passwords, and
  rejects weak or placeholder signing secrets. See
  [Auth](../services/auth/README.md) and
  [gateway authorization](../services/gateway/internal/httpx/auth.go).
- The patient app keeps access tokens in httpOnly cookies and calls the gateway
  server-side. User-profile and feedback contracts avoid medical data and
  unnecessary patient identifiers.
- Prescription documents are not uploaded or stored; prescription-only
  products are identified by catalog metadata and checked by pharmacists at
  dispensing. See the [test plan](test-plan.md).
- Public demo access uses HTTPS through Cloudflare tunnels. Internal gRPC
  traffic currently uses plaintext h2c, including in the Kubernetes cluster;
  therefore encrypted internal connections remain a deployment hardening item.
  See [ADR 0004](adr/0004-traefik-for-discovery-and-load-balancing.md).

### Status log

Add a dated status comment at each sprint review. Record security test results,
new warning signs, and changes to the controls above.
