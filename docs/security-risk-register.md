# Security risk register

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
