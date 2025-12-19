# Pattern: Transactional Outbox

## Intent

Ensure reliable event publishing by storing events in a database table within the same transaction as the business operation, then asynchronously publishing to the message broker.

## Problem

When updating a database and publishing an event, either can fail independently:

```
❌ Problem Scenarios:

1. DB succeeds, publish fails → Event lost
   UPDATE orders SET status = 'confirmed';  ✓
   PUBLISH OrderConfirmed;                   ✗ (broker down)

2. Publish succeeds, DB fails → Duplicate/inconsistent
   PUBLISH OrderConfirmed;                   ✓
   UPDATE orders SET status = 'confirmed';  ✗ (constraint violation)
```

## Solution

Write events to an outbox table within the same database transaction, then publish asynchronously.

```
┌─────────────────────────────────────────────────────────────────┐
│                    Transactional Outbox                          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   DB Transaction                          │   │
│  │                                                           │   │
│  │  1. UPDATE orders SET status = 'confirmed'               │   │
│  │  2. INSERT INTO outbox (event_type, payload, ...)        │   │
│  │                                                           │   │
│  │  COMMIT (atomic)                                          │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   Outbox Processor                        │   │
│  │                                                           │   │
│  │  3. SELECT * FROM outbox WHERE status = 'pending'        │   │
│  │  4. PUBLISH to broker                                     │   │
│  │  5. UPDATE outbox SET status = 'published'               │   │
│  │                                                           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│                     ┌───────────────┐                           │
│                     │ Message Broker│                           │
│                     └───────────────┘                           │
└─────────────────────────────────────────────────────────────────┘
```

## Outbox Table Schema

```sql
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_type  VARCHAR(255) NOT NULL,
    aggregate_id    VARCHAR(255) NOT NULL,
    event_type      VARCHAR(255) NOT NULL,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMP,
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    retry_count     INT NOT NULL DEFAULT 0,
    last_error      TEXT
);

CREATE INDEX idx_outbox_pending ON outbox (status, created_at)
    WHERE status = 'pending';
```

## When to Use

**Good fit:**
- At-least-once delivery required
- Database + broker consistency needed
- Cannot use distributed transactions (2PC)
- Event ordering important per aggregate

**Poor fit:**
- Exactly-once delivery required (impossible)
- Real-time requirements (adds latency)
- Simple operations without side effects

## Implementation in goengine

### Writing to Outbox

```go
// Within a transaction, write business data AND outbox entry
func (s *OrderService) ConfirmOrder(ctx context.Context, orderID string) error {
    return s.db.Transaction(func(tx *sql.Tx) error {
        // 1. Update business entity
        _, err := tx.ExecContext(ctx,
            "UPDATE orders SET status = $1 WHERE id = $2",
            "confirmed", orderID)
        if err != nil {
            return err
        }

        // 2. Write to outbox (same transaction)
        event := OrderConfirmed{
            OrderID:     orderID,
            ConfirmedAt: time.Now(),
        }
        return s.outbox.Write(ctx, tx, event)
    })
}
```

### Outbox Writer

```go
type OutboxWriter struct {
    tableName string
}

func (w *OutboxWriter) Write(ctx context.Context, tx *sql.Tx, event any) error {
    payload, _ := json.Marshal(event)
    eventType := deriveEventType(event)

    _, err := tx.ExecContext(ctx, `
        INSERT INTO outbox (id, event_type, payload, status)
        VALUES ($1, $2, $3, 'pending')
    `, uuid.New(), eventType, payload)

    return err
}
```

### Outbox Processor

```go
type OutboxProcessor struct {
    db        *sql.DB
    publisher Publisher
    interval  time.Duration
}

func (p *OutboxProcessor) Start(ctx context.Context) {
    ticker := time.NewTicker(p.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            p.processBatch(ctx)
        }
    }
}

func (p *OutboxProcessor) processBatch(ctx context.Context) {
    // 1. Fetch pending events
    rows, _ := p.db.QueryContext(ctx, `
        SELECT id, event_type, payload
        FROM outbox
        WHERE status = 'pending'
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED
    `)

    for rows.Next() {
        var id, eventType string
        var payload []byte
        rows.Scan(&id, &eventType, &payload)

        // 2. Publish to broker
        if err := p.publisher.Publish(ctx, eventType, payload); err != nil {
            p.markFailed(ctx, id, err)
            continue
        }

        // 3. Mark as published
        p.markPublished(ctx, id)
    }
}
```

## Delivery Guarantees

| Guarantee | Outbox Support |
|-----------|----------------|
| At-least-once | ✅ Yes (retry on failure) |
| At-most-once | ❌ No (would lose events) |
| Exactly-once | ⚠️ Consumer must deduplicate |

**Consumer deduplication:**
```go
func (h *Handler) Handle(ctx context.Context, event Event) error {
    // Check if already processed
    if h.processed.Contains(event.ID) {
        return nil // Idempotent skip
    }

    // Process event
    if err := h.process(event); err != nil {
        return err
    }

    // Mark as processed
    h.processed.Add(event.ID)
    return nil
}
```

## Related Patterns

- [CQRS](cqrs.md) - Events from command handlers
- [Saga](saga.md) - Reliable saga step execution
- Change Data Capture (CDC) - Alternative approach

## Related ADRs

- [ADR 0009: Transactional Outbox Pattern](../goengine/adr/PRO-0009-transactional-outbox.md)
- [ADR 0025: SQL Event Store](../goengine/adr/PRO-0025-sql-event-store.md)

## Further Reading

- Chris Richardson: [Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html)
- Debezium: [Outbox Pattern](https://debezium.io/blog/2019/02/19/reliable-microservices-data-exchange-with-the-outbox-pattern/)
