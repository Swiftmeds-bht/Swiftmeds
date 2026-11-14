# Notification service

Notification stores user-facing in-app notifications and consumes the event
stream for `hold.confirmed`, `hold.expired`, `hold.rejected`, and
`watchlist.alert`. The shared RabbitMQ consumer claims each `event_id` in
PostgreSQL before handling it, so each event creates at most one notification.

`GET /api/v1/notifications` is exposed by the gateway for the authenticated
patient. Email and SMS are represented by the `ports.Delivery` interface and
use a no-op adapter until a provider is configured.

## Configuration

| Variable | Default |
|---|---|
| `NOTIFICATION_GRPC_PORT` | `50054` |
| `NOTIFICATION_DATABASE_URL` | required |
| `RABBITMQ_URL` | required |
| `LOG_LEVEL` | `info` |
