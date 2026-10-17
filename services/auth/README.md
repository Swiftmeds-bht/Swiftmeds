# Auth service

Auth owns user accounts, bcrypt password hashes, and signed access/refresh
tokens. The gateway calls `ValidateToken` for authenticated requests.

## Contract

[`proto/auth/v1/auth.proto`](../../proto/auth/v1/auth.proto)

| RPC | Purpose |
|---|---|
| `Register` | Create a patient or pharmacist account |
| `Login` | Verify a password and issue access and refresh tokens |
| `RefreshToken` | Rotate a valid refresh token |
| `ValidateToken` | Verify an access token and return user ID and role |

Admins are provisioned out of band and cannot self-register. Access tokens
default to 15 minutes; refresh tokens default to 30 days.

## Configuration

| Variable | Default | Meaning |
|---|---:|---|
| `AUTH_GRPC_PORT` | `50051` | gRPC listen port |
| `AUTH_DATABASE_URL` | required | Postgres connection URL for the `auth` database |
| `JWT_SECRET` | required | HMAC signing secret |
| `JWT_ISSUER` | `swiftmed` | JWT issuer |
| `JWT_TTL` | `15m` | Access token lifetime |

Run migrations in `migrations/001_create_auth_tables.up.sql` before starting.

```bash
go test ./...
go run ./cmd/server
```
