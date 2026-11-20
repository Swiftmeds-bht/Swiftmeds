# Auth service

Auth owns user accounts, bcrypt password hashes, and signed access/refresh
tokens. The gateway calls `ValidateToken` for authenticated requests.

## Contract

[`proto/auth/v1/auth.proto`](../../proto/auth/v1/auth.proto)

| RPC | Purpose |
|---|---|
| `Register` | Create a patient account (staff cannot self-register, SCRUM-123) |
| `Login` | Verify a password and issue access and refresh tokens |
| `RefreshToken` | Rotate a valid refresh token (the old one stops working) |
| `ValidateToken` | Verify an access token and return user ID and role |
| `Logout` | Revoke a refresh token. Safe to call more than once |
| `LinkPharmacy` | Make an account the staff account of the pharmacy it just registered (gateway only) |

A pharmacy owner signs up as a patient. When they register their pharmacy, the
gateway calls `LinkPharmacy` (SCRUM-141): the account becomes a pharmacist of
that pharmacy, and its next refreshed token carries the role and `pharmacy_id`.
The pharmacy starts pending and is invisible until an admin verifies it.
`LinkPharmacy` is idempotent for the same pharmacy and refuses admins and
accounts already linked to another pharmacy.

Admins are provisioned out of band and cannot self-register. Passwords must be
8 to 72 bytes (72 is bcrypt's limit). Access tokens default to 15 minutes;
refresh tokens default to 30 days.

| Error | gRPC code |
|---|---|
| Invalid email, password length or role; admin self-registration | `INVALID_ARGUMENT` |
| Email already registered | `ALREADY_EXISTS` |
| `LinkPharmacy` for an unknown user | `NOT_FOUND` |
| `LinkPharmacy` for an admin, or an account linked to another pharmacy | `FAILED_PRECONDITION` |
| Wrong email or password; invalid, expired or revoked token | `UNAUTHENTICATED` |
| Database or other server failure | `INTERNAL` (details only in the logs) |

## Configuration

| Variable | Default | Meaning |
|---|---:|---|
| `AUTH_GRPC_PORT` | `50051` | gRPC listen port |
| `AUTH_DATABASE_URL` | required | Postgres connection URL for `auth_db` |
| `JWT_SECRET` | required | HMAC signing secret |
| `JWT_ISSUER` | `swiftmed` | JWT issuer |
| `JWT_TTL` | `15m` | Access token lifetime |
| `JWT_REFRESH_TTL` | `720h` | Refresh token lifetime |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

Database migrations in `migrations/` are applied automatically at startup
(`pkg/migrate`). To add one, create the next `00N_name.up.sql` file.

## Run and test

```bash
go test -cover ./...
go run ./cmd/server          # needs AUTH_DATABASE_URL and JWT_SECRET
```

In the full stack (`make up`) auth is reached through Traefik:

```bash
grpcurl -plaintext -authority auth localhost:50051 list swiftmed.auth.v1.AuthService
```
