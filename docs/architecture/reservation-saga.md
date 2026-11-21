# Reservation saga

Back to [Architecture](README.md). Why it is a choreography: [ADR 0002](../adr/0002-choreographed-saga.md).

A hold reserves units of one product at one pharmacy for a short time (2 hours by
default, `HOLDS_HOLD_DURATION` and `EXPIRY_HOLD_DURATION`). Holds owns the hold and
its status; Inventory owns the stock. No service tells the others what to do: Holds
and Inventory react to each other's events on the `swiftmed.events` exchange, and
the steps that undo a reservation are direct gRPC calls to Inventory.

Code: `services/holds/internal/app/service.go` and `saga.go`,
`services/inventory/internal/app/service.go`,
`services/inventory/internal/adapters/postgres/reservation.go`,
`services/expiry/internal/app/service.go`.
Tests: `tests/integration/saga` (real Holds and Inventory, Postgres and RabbitMQ in
containers; the "Saga integration tests" CI job).

## Hold states

```mermaid
stateDiagram-v2
    [*] --> PENDING: CreateHold, hold.requested published
    PENDING --> ACTIVE: stock.reserved
    PENDING --> REJECTED: stock.unavailable, or hold.requested could not be published
    PENDING --> CANCELLED: patient cancels
    ACTIVE --> COLLECTED: CollectHold with the pickup code, then CommitReservation
    ACTIVE --> CANCELLED: patient cancels, after ReleaseReservation
    ACTIVE --> EXPIRED: Expiry sweep, after ReleaseReservation
    REJECTED --> [*]
    CANCELLED --> [*]
    EXPIRED --> [*]
    COLLECTED --> [*]
```

Inventory keeps its own record per hold in `inventory_reservations`, keyed by
`hold_id`: `UNAVAILABLE`, `RESERVED`, then `COMMITTED` or `RELEASED`. Only one of
commit and release can win, which settles races such as cancel against collect.

## 1. Happy path: stock reserved, hold confirmed

```mermaid
sequenceDiagram
    autonumber
    actor P as Patient
    participant App as Patient app
    participant GW as Gateway
    participant H as Holds
    participant MQ as RabbitMQ swiftmed.events
    participant I as Inventory
    participant E as Expiry

    P->>App: Reserve (product, pharmacy, quantity)
    App->>GW: POST /api/v1/holds (Bearer token from cookie)
    GW->>H: gRPC CreateHold
    H->>H: store hold PENDING (max 3 open holds per patient)
    H->>MQ: publish hold.requested
    H-->>GW: hold PENDING
    GW-->>App: 202 hold PENDING
    MQ->>I: hold.requested (queue inventory.hold-requested)
    I->>I: one transaction: record reservation by hold_id, available minus quantity
    I->>MQ: publish stock.reserved
    MQ->>H: stock.reserved (queue holds.stock-events)
    H->>H: PENDING to ACTIVE, pickup code, expires_at
    H->>MQ: publish hold.confirmed
    MQ->>E: hold.confirmed (queue expiry.hold-confirmed)
    E->>E: store the deadline
    loop every 2 s until the status changes
        App->>GW: GET /api/v1/holds/{holdId}
        GW->>H: gRPC GetHold
        H-->>App: ACTIVE with pickup code
    end
```

Every event carries `metadata.correlation_id` = the hold ID. Each consumer
deduplicates on the event ID, so a redelivered event changes nothing (test
`TestDuplicateEventsChangeNothing`).

## 2. No stock: hold rejected

```mermaid
sequenceDiagram
    autonumber
    participant H as Holds
    participant MQ as RabbitMQ
    participant I as Inventory

    H->>MQ: publish hold.requested
    MQ->>I: hold.requested
    I->>I: record reservation UNAVAILABLE, stock unchanged
    I->>MQ: publish stock.unavailable (requested and available quantity)
    MQ->>H: stock.unavailable
    H->>H: PENDING to REJECTED, reason "stock unavailable"
    Note over H,I: Nothing was reserved, so nothing is released.
```

If Holds cannot publish `hold.requested` at all, it rejects the hold at once
(reason "stock request could not be sent") so it does not count against the
patient's limit, and the gateway reports the error.

## 3. Compensation: the patient cancels an active hold

```mermaid
sequenceDiagram
    autonumber
    participant App as Patient app
    participant GW as Gateway
    participant H as Holds
    participant I as Inventory

    App->>GW: POST /api/v1/holds/{holdId}/cancel
    GW->>H: gRPC CancelHold
    H->>H: check owner and status ACTIVE
    H->>I: gRPC ReleaseReservation(hold_id) through Traefik
    I->>I: RESERVED to RELEASED, units back to available
    I-->>H: ok
    H->>H: ACTIVE to CANCELLED
    H-->>App: hold CANCELLED
    Note over H,I: If ReleaseReservation fails, the hold stays ACTIVE and the patient can retry.
```

Inventory is called first and the hold is stored second, so a failed call never
leaves a cancelled hold holding stock. Releasing twice is safe: a reservation that
is already `RELEASED` (or was `UNAVAILABLE`) is returned unchanged.

## 4. Compensation: the hold expires

```mermaid
sequenceDiagram
    autonumber
    participant W as Worker hold-expiry
    participant GW as Gateway
    participant E as Expiry
    participant H as Holds
    participant I as Inventory

    W->>GW: POST /api/v1/internal/expiry/sweep (X-Worker-Secret, asOf), every minute
    GW->>E: gRPC RunSweep(asOf)
    loop each deadline that has passed (up to EXPIRY_SWEEP_BATCH_SIZE)
        E->>H: gRPC ExpireHold(hold_id)
        H->>I: gRPC ReleaseReservation(hold_id)
        I-->>H: ok
        H->>H: ACTIVE to EXPIRED
        H-->>E: hold EXPIRED
    end
    E-->>GW: expired hold IDs
    GW-->>W: 200 expiredHoldIds
```

`ExpireHold` does nothing for a hold that is no longer ACTIVE, and the next sweep
retries anything that failed, so a sweep is safe to repeat (test
`TestExpiredHoldReleasesItsStock`).

## 5. Compensation: the stock was reserved after the patient cancelled

A patient can cancel while the hold is still PENDING. Holds cancels it at once;
if Inventory's `stock.reserved` arrives afterwards, Holds gives the stock back.

```mermaid
sequenceDiagram
    autonumber
    participant App as Patient app
    participant H as Holds
    participant MQ as RabbitMQ
    participant I as Inventory

    H->>MQ: publish hold.requested
    App->>H: CancelHold (via gateway) while PENDING
    H->>H: PENDING to CANCELLED, nothing to release yet
    MQ->>I: hold.requested
    I->>I: reserve units
    I->>MQ: publish stock.reserved
    MQ->>H: stock.reserved
    H->>H: hold is CANCELLED, so this reservation is late
    H->>I: gRPC ReleaseReservation(hold_id)
    I->>I: RESERVED to RELEASED
```

The same applies to a hold that is EXPIRED or REJECTED when `stock.reserved`
arrives (`HandleStockReserved` in `saga.go`).

## 6. Collecting

At the counter, `POST /api/v1/holds/{holdId}/collect` with the pickup code calls
`CollectHold`. Holds checks the pharmacy and code, calls Inventory
`CommitReservation` (the reserved units leave stock for good), then stores
COLLECTED. A wrong code leaves the hold ACTIVE.

## Failure handling

| Failure | What happens |
|---|---|
| A consumer's handler fails | `pkg/messaging` retries the delivery up to 3 times, then dead-letters it to `<queue>.dlq` |
| The same event arrives twice | Skipped by the deduplication table; Inventory's reservation is also keyed by `hold_id` |
| Holds publishes `hold.confirmed` but the publish fails | The `stock.reserved` delivery fails and is retried; the hold is already ACTIVE, so the retry only publishes `hold.confirmed` again |
| Inventory is unreachable on cancel or expire | The hold keeps its status; the patient retries the cancel, or the next sweep retries the expiry |
| Cancel and collect at the same moment | Inventory allows only one of release and commit per reservation; the hold update also checks the previous status |
