# Transactional Outbox Pattern Overview

This document provides a high-level overview of the Transactional Outbox pattern and how it will be implemented in gopipe.

## What is the Transactional Outbox Pattern?

The **Transactional Outbox Pattern** ensures **exactly-once message delivery** by storing outbound messages in a database table within the same transaction as the business data.

### The Problem: Dual Writes

In distributed systems, a common anti-pattern is performing two separate write operations:

```
❌ Problem:
1. Write to database   ← Transaction 1
2. Publish to broker   ← Transaction 2 (separate!)

What if one fails?
  - DB succeeds, broker fails → Data inconsistency
  - Broker succeeds, DB fails → Duplicate processing
  - Retry → Duplicate messages
```

### The Solution: Outbox Table

Store both the business data **and** the outbound message in a single database transaction:

```
✅ Solution:
┌─── Single Transaction ───┐
│ 1. INSERT INTO orders    │
│ 2. INSERT INTO outbox    │ ← Same transaction!
│ COMMIT (atomic)          │
└──────────────────────────┘
        ↓
Background Processor
        ↓
Publish messages to broker
```

## How It Works

### 1. Write Phase (Application)

```sql
BEGIN TRANSACTION;

-- Business logic
INSERT INTO orders (id, customer_id, amount)
VALUES ('order-123', 'customer-456', 100);

-- Store message in outbox
INSERT INTO message_outbox (payload, properties)
VALUES ('{"id":"order-123",...}', '{"subject":"OrderCreated"}');

COMMIT; -- Both writes succeed or both fail (atomic!)
```

### 2. Publish Phase (Background Processor)

```
Background Processor (runs every 1 second):

1. SELECT * FROM message_outbox
   WHERE published_at IS NULL
   LIMIT 100;

2. For each message:
   a. Publish to broker
   b. If successful:
      UPDATE message_outbox
      SET published_at = NOW()
      WHERE id = ?

3. Cleanup old messages:
   DELETE FROM message_outbox
   WHERE published_at < NOW() - INTERVAL '24 hours'
```

## Benefits

### 1. Exactly-Once Delivery Guarantee

```
✅ Database committed + Outbox stored → Message will be published
✅ Database rolled back → Outbox empty, no message
✅ Publish failed → Auto-retry by processor
✅ Publish succeeded → Marked published, no duplicate
```

### 2. Atomic Consistency

```
Single database transaction ensures:
- Business data saved ⇔ Message stored
- No partial writes
- No inconsistency
```

### 3. Resilience

```
Scenario 1: Broker down
  → Messages accumulate in outbox
  → Auto-retry when broker recovers

Scenario 2: Application crashes
  → Outbox persisted in database
  → Processor resumes on restart

Scenario 3: Network issues
  → Retry with exponential backoff
  → Eventually published
```

## gopipe Implementation (Proposed)

**Status:** 📋 Proposed ([ADR 0009](./adr/0009-transactional-outbox-pattern.md))

### Database Schema

```sql
CREATE TABLE message_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payload BYTEA NOT NULL,
    properties JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Efficient pending message queries
CREATE INDEX idx_message_outbox_pending
    ON message_outbox(created_at)
    WHERE published_at IS NULL;

-- Cleanup of old messages
CREATE INDEX idx_message_outbox_published
    ON message_outbox(published_at)
    WHERE published_at IS NOT NULL;
```

### Core Interfaces

```go
package outbox

// OutboxStore persists outbound messages transactionally
type OutboxStore interface {
    // Store saves a message within a transaction
    Store(ctx context.Context, tx Transaction, msg *message.Message) error

    // GetPending retrieves unpublished messages
    GetPending(ctx context.Context, limit int) ([]*OutboxMessage, error)

    // MarkPublished marks a message as published
    MarkPublished(ctx context.Context, id string) error

    // MarkFailed records a failed publish attempt
    MarkFailed(ctx context.Context, id string, err error) error

    // DeleteOld removes old published messages
    DeleteOld(ctx context.Context, olderThan time.Duration) error
}

// OutboxProcessor publishes messages from the outbox
type OutboxProcessor struct {
    store     OutboxStore
    publisher Publisher
    config    OutboxConfig
}

type OutboxConfig struct {
    PollInterval    time.Duration // How often to check (default: 1s)
    BatchSize       int           // Messages per batch (default: 100)
    MaxRetries      int           // Max publish attempts (default: 10)
    RetryBackoff    BackoffFunc   // Retry strategy
    CleanupInterval time.Duration // Cleanup frequency (default: 1h)
    CleanupAge      time.Duration // Age threshold (default: 24h)
}
```

### Usage Example

```go
// 1. Setup
db, _ := sql.Open("postgres", "...")
outboxStore := outbox.NewPostgresOutboxStore(db)
publisher := &MyBrokerPublisher{client: natsClient}

processor := outbox.NewOutboxProcessor(
    outboxStore,
    publisher,
    outbox.OutboxConfig{
        PollInterval:    1 * time.Second,
        BatchSize:       100,
        MaxRetries:      10,
        RetryBackoff:    outbox.ExponentialBackoff(1*time.Second, 1*time.Minute),
        CleanupInterval: 1 * time.Hour,
        CleanupAge:      24 * time.Hour,
    },
)

go processor.Start(context.Background())
defer processor.Stop()

// 2. Use in command handler
func handleCreateOrder(ctx context.Context, cmd CreateOrder) error {
    tx, _ := db.BeginTx(ctx, nil)
    defer tx.Rollback()

    // Business logic
    _, err := tx.ExecContext(ctx,
        `INSERT INTO orders (id, customer_id, amount)
         VALUES ($1, $2, $3)`,
        cmd.ID, cmd.CustomerID, cmd.Amount,
    )
    if err != nil {
        return err
    }

    // Store event in outbox (same transaction!)
    evt := OrderCreated{ID: cmd.ID, Amount: cmd.Amount}
    payload, _ := json.Marshal(evt)
    msg := message.New(payload, message.Properties{
        message.PropSubject: "OrderCreated",
    })

    if err := outboxStore.Store(ctx, tx, msg); err != nil {
        return err
    }

    // Commit both atomically
    return tx.Commit()
}
```

## Outbox vs Alternatives

### Outbox vs At-Least-Once + Idempotency

| Aspect | Outbox | At-Least-Once |
|--------|--------|---------------|
| **Delivery** | Exactly-once | At-least-once |
| **Complexity** | Higher | Lower |
| **Performance** | Overhead (DB writes) | Better |
| **Duplicates** | Prevented | Possible |
| **Consumer req** | None | Must be idempotent |

**When to use outbox:**
- ✅ Can't make consumers idempotent
- ✅ Critical workflows (payments)
- ✅ Duplicates unacceptable

**When to use at-least-once:**
- ✅ Consumers idempotent
- ✅ Better performance needed
- ✅ Occasional duplicates OK

### Outbox vs Change Data Capture (CDC)

| Aspect | Outbox | CDC |
|--------|--------|-----|
| **Implementation** | Application code | Database plugin |
| **Control** | Full | Limited |
| **Event format** | Custom | Table schema |
| **Setup** | Simple | Complex |
| **Maintenance** | Low | Medium |

**When to use outbox:**
- ✅ Want full control over events
- ✅ Simple setup
- ✅ Custom event formats

**When to use CDC:**
- ✅ No code changes
- ✅ Replicate all DB changes
- ✅ Infrastructure team manages

### Outbox vs Broker Transactions

Some brokers support transactions (e.g., Kafka).

| Aspect | Outbox | Broker TX |
|--------|--------|-----------|
| **Atomicity** | DB + outbox | Producer + broker |
| **Portability** | Any broker | Broker-specific |
| **Complexity** | Medium | High |

**When to use outbox:**
- ✅ Broker doesn't support TX
- ✅ Want portability
- ✅ Simpler mental model

## Performance Considerations

### Latency

**Write latency:** Same as normal (single DB transaction)
**Delivery latency:** Delayed by poll interval (default: 1s)

```
Traditional:
Command → DB write → Broker publish → Event delivery
                     (immediate)

Outbox:
Command → DB write (order + outbox) → Processor polls → Broker publish → Event delivery
                                      (1s delay)
```

**Tradeoff:** Consistency vs. low latency

**Optimization:**
```go
// Reduce latency
config := OutboxConfig{
    PollInterval: 100 * time.Millisecond, // Lower delay
}
```

### Throughput

**Database:**
- One additional INSERT per message
- Indexes maintained

**Processor:**
- Batch processing (configurable)
- Parallel workers possible

**Optimization:**
```go
// Higher throughput
config := OutboxConfig{
    BatchSize: 1000,  // Process more per batch
}
```

### Storage

**Growth:** Outbox table grows over time

**Cleanup:** Background job removes old messages

**Optimization:**
```go
// Aggressive cleanup
config := OutboxConfig{
    CleanupInterval: 15 * time.Minute,
    CleanupAge:      1 * time.Hour,
}

// Or use partitioning
CREATE TABLE message_outbox (...)
PARTITION BY RANGE (created_at);
```

## Best Practices

### 1. Use Idempotency Keys

Even with outbox, consumers should be idempotent:

```go
// Message should include idempotency key
props := message.Properties{
    message.PropSubject:       "OrderCreated",
    message.PropCorrelationID: "corr-123",
    "idempotency_key":         "order-123-created",  // ✅ Idempotency key
}
```

### 2. Monitor Outbox Size

```sql
-- Alert if outbox grows too large
SELECT COUNT(*) FROM message_outbox WHERE published_at IS NULL;

-- Should typically be < 1000
```

### 3. Handle Poison Messages

```sql
-- Find messages that keep failing
SELECT * FROM message_outbox
WHERE attempts >= 10
AND published_at IS NULL;

-- Move to dead letter queue
INSERT INTO dead_letter_queue SELECT * FROM message_outbox WHERE attempts >= 10;
DELETE FROM message_outbox WHERE attempts >= 10;
```

### 4. Scale the Processor

```go
// Run multiple processors for throughput
for i := 0; i < numWorkers; i++ {
    processor := outbox.NewOutboxProcessor(...)
    go processor.Start(ctx)
}
```

Use `SELECT ... FOR UPDATE SKIP LOCKED` to prevent concurrent processing.

### 5. Test Failure Scenarios

```go
func TestOutboxFailureScenarios(t *testing.T) {
    // Scenario 1: DB commit fails
    tx.Rollback()
    // Verify: No message in outbox ✅

    // Scenario 2: Broker publish fails
    mockBroker.SetFail(true)
    // Verify: Message stays in outbox, retried ✅

    // Scenario 3: Application crashes
    processor.Stop()
    // Verify: Messages processed after restart ✅
}
```

## When to Use Transactional Outbox

### Use Outbox When:

✅ Need exactly-once delivery guarantee
✅ Database writes + message publishing must be atomic
✅ Can't tolerate duplicate events
✅ Critical business operations (payments, orders, inventory)
✅ Have database transactions available
✅ Latency overhead (poll interval) acceptable

### Don't Use Outbox When:

❌ At-least-once delivery sufficient
❌ Consumers are idempotent (handle duplicates)
❌ No database (event-sourced systems)
❌ Ultra-low latency required
❌ Broker has native exactly-once support
❌ Simple CRUD operations

## Common Patterns

### Pattern 1: CQRS + Outbox

```
Command Handler:
┌─── Transaction ────┐
│ Save aggregate     │
│ Store events       │ ← Outbox
│ COMMIT             │
└────────────────────┘
```

### Pattern 2: Saga + Outbox

```
Saga Step:
┌─── Transaction ────┐
│ Update saga state  │
│ Store commands     │ ← Outbox
│ COMMIT             │
└────────────────────┘
```

### Pattern 3: Event Sourcing + Outbox

```
Event Store Write:
┌─── Transaction ────┐
│ Append to events   │
│ Store notification │ ← Outbox
│ COMMIT             │
└────────────────────┘
```

## Migration Path

### Step 1: Set up infrastructure

```go
// Create outbox table
db.Exec(outboxSchema)

// Start processor
outboxStore := outbox.NewPostgresOutboxStore(db)
processor := outbox.NewOutboxProcessor(...)
go processor.Start(ctx)
```

### Step 2: Migrate critical handlers

```go
// Before: Direct publish
func handleCreateOrder(ctx, cmd) error {
    db.SaveOrder(cmd)
    broker.Publish(OrderCreated{...})  // ❌ Dual write
}

// After: Outbox
func handleCreateOrder(ctx, cmd) error {
    tx, _ := db.BeginTx(ctx, nil)
    defer tx.Rollback()

    db.SaveOrderTx(tx, cmd)
    outboxStore.Store(ctx, tx, OrderCreated{...})

    tx.Commit()  // ✅ Atomic
}
```

### Step 3: Keep non-critical handlers as-is

```go
// Analytics events can stay at-least-once
analyticsHandler := cqrs.NewEventHandler(...) // No outbox needed
```

## Related Documentation

### Architecture Decision Records

- [ADR 0006: CQRS Implementation](./adr/0006-cqrs-implementation.md) ✅ Implemented
- [ADR 0007: Saga Coordinator Pattern](./adr/0007-saga-coordinator-pattern.md) ✅ Implemented
- [ADR 0008: Compensating Saga Pattern](./adr/0008-compensating-saga-pattern.md) 📋 Proposed
- [ADR 0009: Transactional Outbox Pattern](./adr/0009-transactional-outbox-pattern.md) 📋 Proposed

### Pattern Guides

- [CQRS Overview](./cqrs-overview.md)
- [Saga Pattern Overview](./saga-overview.md)
- [CQRS Architecture Overview](./cqrs-architecture-overview.md)
- [Advanced Patterns Documentation](./cqrs-advanced-patterns.md)

### External Resources

- [Transactional Outbox - Microservices.io](https://microservices.io/patterns/data/transactional-outbox.html)
- [Outbox Pattern - Debezium](https://debezium.io/documentation/reference/transformations/outbox-event-router.html)
- [Outbox Pattern - Chris Richardson](https://chrisrichardson.net/post/microservices/patterns/2020/04/27/transactional-outbox-polling.html)
- [Exactly-Once Delivery - Uber Engineering](https://www.uber.com/blog/reliable-reprocessing/)
