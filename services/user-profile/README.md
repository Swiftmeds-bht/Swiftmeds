# User Profile service

User Profile stores the display details associated with an Auth user ID. It
does not store medical data.

## Contract

[`proto/user-profile/v1/user_profile.proto`](../../proto/user-profile/v1/user_profile.proto)

| RPC | Purpose |
|---|---|
| `GetProfile` | Retrieve a profile by Auth user ID |
| `UpdateProfile` | Create or update a profile |

The application layer also exposes `CreateProfile`; the frozen gRPC contract
uses `UpdateProfile` as an idempotent create-or-update operation.

## Configuration

| Variable | Default | Meaning |
|---|---:|---|
| `USER_PROFILE_GRPC_PORT` | `50052` | gRPC listen port |
| `USER_PROFILE_DATABASE_URL` | required | Postgres connection URL for `user_profile_db` |
| `LOG_LEVEL` | `info` | Log level |

## Run and test

```bash
go test -cover ./...
go run ./cmd/server
```

Through Traefik:

```bash
grpcurl -plaintext -authority user-profile localhost:50051 list swiftmed.userprofile.v1.UserProfileService
```
