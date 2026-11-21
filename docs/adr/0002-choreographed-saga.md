# 0002. Choreographed saga for reservations, not orchestration

- **Status:** Accepted
- **Date:** 2026-11-11 (recorded after the saga and its integration tests, SCRUM-56, were built)
- **Deciders:** SwiftMed team, recorded by the architecture lead

## Context

Reserving a medicine changes data in two services that each own their database
([ADR 0003](0003-database-per-service.md)): Holds owns the hold and its pickup
code, Inventory owns the stock. There is no shared transaction. A reservation
must never leave stock reserved for a hold that no longer exists, and must never
confirm a hold without stock behind it, even when messages are redelivered,
arrive late, or a service restarts half way through.

The flow is short (one request, one answer, then possibly one undo) and involves
only two services, with Expiry, Notification and Analytics as listeners.

## Decision

Run the reservation as a **choreographed saga** over RabbitMQ (topic exchange
`swiftmed.events`), with no central coordinator:

1. Holds stores the hold as PENDING and publishes `hold.requested`.
2. Inventory reserves atomically, using the hold ID as the idempotency key
   (`inventory_reservations.hold_id`), and publishes `stock.reserved` or
   `stock.unavailable`.
3. Holds makes the hold ACTIVE (pickup code, expiry time) and publishes
   `hold.confirmed`, or makes it REJECTED.
4. Compensation is a direct gRPC call from Holds to Inventory
   `ReleaseReservation` when an ACTIVE hold is cancelled or expired, or when
   `stock.reserved` arrives for a hold that has already ended. Collecting calls
   `CommitReservation`. Inventory allows only one of release and commit per hold.

Every consumer is idempotent: `pkg/messaging` skips event IDs it has already
processed, retries a failing handler 3 times, then dead-letters the delivery; and
every saga step in `services/holds/internal/app/saga.go` is safe to repeat.

The sequence diagrams are in [docs/architecture/reservation-saga.md](../architecture/reservation-saga.md).

## Consequences

- Holds and Inventory do not call each other to start a reservation, so a slow or
  restarting Inventory only delays holds (they stay PENDING) instead of failing
  the patient's request.
- New listeners (Expiry today; Notification and Analytics later) subscribe to
  `hold.confirmed` without any change to Holds or Inventory.
- The flow is not written down in one place in the code; it is spread over two
  services' handlers. This document set, the event comments in
  `events.proto`, and the integration tests in `tests/integration/saga` (happy
  path, unavailable, expiry, cancel, duplicate events) are what keep it
  understandable.
- The client cannot get the final answer in the create call: `POST /api/v1/holds`
  returns 202 with a PENDING hold and the patient app polls every 2 seconds.
- Compensation uses synchronous gRPC, not an event, so a cancel or expiry fails
  (and is retried) while Inventory is down, instead of being queued. That keeps the
  rule "the hold is stored only after Inventory agreed" simple.

## Alternatives considered

- **Orchestration** (a saga coordinator, or Holds calling Inventory and driving
  every step). One place to read the flow, but an extra stateful component or a
  tight synchronous coupling for a two-step flow, and the coordinator becomes the
  thing that must not lose its state.
- **Synchronous reservation in the create call** (Holds calls Inventory `Reserve`
  over gRPC). Simpler for the client, but a create then fails whenever Inventory
  is slow, and partial failures (reserved but the hold not saved) still need
  compensation.
- **Two-phase commit across the two databases.** Not supported by our stack
  without a transaction manager, and it couples the two services' availability.
