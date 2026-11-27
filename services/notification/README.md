# Notification service

Notification stores user-facing in-app notifications and consumes the event
stream for `hold.confirmed`, `hold.expired`, `hold.rejected`, and
`watchlist.alert`. The shared RabbitMQ consumer claims each `event_id` in
PostgreSQL before handling it, so each event creates at most one notification.

`GET /api/v1/notifications` is exposed by the gateway for authenticated users.
Notifications are in-app only: email and SMS delivery are not supported, and
the notification service does not read profile contact details.

## Configuration

| Variable | Default |
|---|---|
| `NOTIFICATION_GRPC_PORT` | `50054` |
| `NOTIFICATION_DATABASE_URL` | required |
| `RABBITMQ_URL` | required |
| `LOG_LEVEL` | `info` |
