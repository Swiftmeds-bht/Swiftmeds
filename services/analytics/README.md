# Analytics

Analytics consumes `search.recorded` and `stock.unavailable` events and exposes
unmet-demand aggregates. It stores only event identifiers and aggregate inputs;
no user data is persisted. Stock-unavailable events resolve pharmacy district
and product name through their gRPC services before persistence, so lookup
failures are retried by RabbitMQ.

Run with the repository Compose stack. Configuration requires
`ANALYTICS_DATABASE_URL`, `RABBITMQ_URL`, and `ANALYTICS_UPSTREAM`.
