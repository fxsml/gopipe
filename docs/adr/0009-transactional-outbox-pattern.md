# ADR 0009: Transactional Outbox Pattern

**Date:** 2025-12-08
**Status:** Proposed
**Related:**
- [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)
- [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md)

## Context

In distributed systems, a common challenge is ensuring **exactly-once message delivery** when publishing messages as part of a database transaction.

### The Problem: Dual Writes

```go
// ❌ Problem: Two separate operations, not atomic!
func CreateOrder(ctx context.Context, order Order) error {
    // Write 1: Database
    if err := db.SaveOrder(ctx, order); err != nil {
        return err
    }

    // Write 2: Message broker
    if err := broker.Publish(ctx, OrderCreated{...}); err != nil {
        // Database already committed!
        // Now we have inconsistency!
        return err
    }

    return nil
}
```

**Failure Scenarios:**

1. **Database succeeds, broker fails**
   - Order saved but no event published
   - Downstream services never notified
   - Data inconsistency

2. **Broker succeeds, database fails**
   - Event published but order not saved
   - Downstream services process non-existent order
   - Data inconsistency

3. **Retry after partial failure**
   - Duplicate events published
   - At-least-once (not exactly-once) delivery

### Solution: Transactional Outbox

Write both the database record **and** the outbound message in a **single database transaction**:

```
┌─── Database Transaction ────┐
│ 1. INSERT INTO orders       │
│ 2. INSERT INTO outbox       │
│ COMMIT (atomic!)            │
└─────────────────────────────┘
         ↓
Background Outbox Processor
         ↓
┌─────────────────────────────┐
│ SELECT * FROM outbox        │
│ WHERE published_at IS NULL  │
│     ↓                       │
│ Publish to broker           │
│     ↓                       │
│ UPDATE outbox               │
│ SET published_at = NOW()    │
└─────────────────────────────┘
```

## Decision

Implement the **Transactional Outbox Pattern** as an optional, pluggable package: `cqrs/outbox`.

### Design Principles

1. **Optional** - Only needed for exactly-once semantics (<1% of use cases)
2. **Pluggable** - Can be added to existing code incrementally
3. **Storage-agnostic** - `OutboxStore` interface supports any database
4. **Reliable** - Background processor with retry and error handling
5. **Observable** - Metrics and monitoring built-in

## Proposed Architecture

### 1. Core Interface: OutboxStore

```go
package outbox

// OutboxStore persists outbound messages in a transactional outbox.
type OutboxStore interface {
    // Store saves a message to the outbox within a transaction.
    // The message will be published by the background processor.
    Store(ctx context.Context, tx Transaction, msg *message.Message) error

    // GetPending retrieves unpublished messages from the outbox.
    // Limit controls how many messages to fetch per batch.
    GetPending(ctx context.Context, limit int) ([]*OutboxMessage, error)

    // MarkPublished marks a message as successfully published.
    MarkPublished(ctx context.Context, id string) error

    // MarkFailed marks a message as failed after max retries.
    MarkFailed(ctx context.Context, id string, err error) error

    // DeleteOld deletes published messages older than the given duration.
    DeleteOld(ctx context.Context, olderThan time.Duration) error
}

// Transaction represents a database transaction.
// The exact type depends on the database driver (e.g., *sql.Tx, pgx.Tx).
type Transaction interface{}

// OutboxMessage represents a message stored in the outbox.
type OutboxMessage struct {
    ID          string
    Payload     []byte
    Properties  message.Properties
    CreatedAt   time.Time
    PublishedAt *time.Time
    Attempts    int
    LastError   string
}
```

### 2. PostgreSQL Schema

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

-- Index for efficient pending message queries
CREATE INDEX idx_message_outbox_pending
    ON message_outbox(created_at)
    WHERE published_at IS NULL;

-- Index for cleanup of old messages
CREATE INDEX idx_message_outbox_published
    ON message_outbox(published_at)
    WHERE published_at IS NOT NULL;
```

### 3. PostgreSQL Implementation

```go
// PostgresOutboxStore implements OutboxStore for PostgreSQL.
type PostgresOutboxStore struct {
    db *sql.DB
}

func NewPostgresOutboxStore(db *sql.DB) OutboxStore {
    return &PostgresOutboxStore{db: db}
}

func (s *PostgresOutboxStore) Store(
    ctx context.Context,
    tx Transaction,
    msg *message.Message,
) error {
    sqlTx := tx.(*sql.Tx)

    propsJSON, err := json.Marshal(msg.Properties)
    if err != nil {
        return fmt.Errorf("marshal properties: %w", err)
    }

    _, err = sqlTx.ExecContext(ctx,
        `INSERT INTO message_outbox (payload, properties)
         VALUES ($1, $2)`,
        msg.Payload,
        propsJSON,
    )

    return err
}

func (s *PostgresOutboxStore) GetPending(
    ctx context.Context,
    limit int,
) ([]*OutboxMessage, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, payload, properties, created_at, attempts, last_error
         FROM message_outbox
         WHERE published_at IS NULL
         ORDER BY created_at ASC
         LIMIT $1
         FOR UPDATE SKIP LOCKED`, // Prevent concurrent processing
        limit,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []*OutboxMessage
    for rows.Next() {
        var msg OutboxMessage
        var propsJSON []byte

        err := rows.Scan(
            &msg.ID,
            &msg.Payload,
            &propsJSON,
            &msg.CreatedAt,
            &msg.Attempts,
            &msg.LastError,
        )
        if err != nil {
            return nil, err
        }

        if err := json.Unmarshal(propsJSON, &msg.Properties); err != nil {
            return nil, err
        }

        messages = append(messages, &msg)
    }

    return messages, rows.Err()
}

func (s *PostgresOutboxStore) MarkPublished(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE message_outbox
         SET published_at = NOW(), updated_at = NOW()
         WHERE id = $1`,
        id,
    )
    return err
}

func (s *PostgresOutboxStore) MarkFailed(
    ctx context.Context,
    id string,
    err error,
) error {
    _, dbErr := s.db.ExecContext(ctx,
        `UPDATE message_outbox
         SET attempts = attempts + 1,
             last_error = $2,
             updated_at = NOW()
         WHERE id = $1`,
        id,
        err.Error(),
    )
    return dbErr
}

func (s *PostgresOutboxStore) DeleteOld(
    ctx context.Context,
    olderThan time.Duration,
) error {
    _, err := s.db.ExecContext(ctx,
        `DELETE FROM message_outbox
         WHERE published_at IS NOT NULL
         AND published_at < NOW() - $1::INTERVAL`,
        olderThan.String(),
    )
    return err
}
```

### 4. Outbox Processor (Background Worker)

```go
// OutboxProcessor continuously polls the outbox and publishes messages.
type OutboxProcessor struct {
    store       OutboxStore
    publisher   Publisher
    config      OutboxConfig
    stopCh      chan struct{}
    stoppedCh   chan struct{}
}

type OutboxConfig struct {
    // PollInterval is how often to check for pending messages
    PollInterval time.Duration // Default: 1s

    // BatchSize is how many messages to process per batch
    BatchSize int // Default: 100

    // MaxRetries is the maximum number of publish attempts
    MaxRetries int // Default: 10

    // RetryBackoff is the backoff strategy for retries
    RetryBackoff BackoffFunc

    // CleanupInterval is how often to delete old published messages
    CleanupInterval time.Duration // Default: 1h

    // CleanupAge is how old published messages must be before deletion
    CleanupAge time.Duration // Default: 24h
}

// Publisher sends messages to a broker.
type Publisher interface {
    Publish(ctx context.Context, msg *message.Message) error
}

func NewOutboxProcessor(
    store OutboxStore,
    publisher Publisher,
    config OutboxConfig,
) *OutboxProcessor {
    return &OutboxProcessor{
        store:     store,
        publisher: publisher,
        config:    config,
        stopCh:    make(chan struct{}),
        stoppedCh: make(chan struct{}),
    }
}

// Start begins processing the outbox in the background.
func (p *OutboxProcessor) Start(ctx context.Context) {
    defer close(p.stoppedCh)

    pollTicker := time.NewTicker(p.config.PollInterval)
    defer pollTicker.Stop()

    cleanupTicker := time.NewTicker(p.config.CleanupInterval)
    defer cleanupTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-p.stopCh:
            return
        case <-pollTicker.C:
            p.processBatch(ctx)
        case <-cleanupTicker.C:
            p.cleanup(ctx)
        }
    }
}

func (p *OutboxProcessor) processBatch(ctx context.Context) {
    messages, err := p.store.GetPending(ctx, p.config.BatchSize)
    if err != nil {
        log.Printf("ERROR: Failed to get pending messages: %v", err)
        return
    }

    for _, outboxMsg := range messages {
        p.processMessage(ctx, outboxMsg)
    }
}

func (p *OutboxProcessor) processMessage(
    ctx context.Context,
    outboxMsg *OutboxMessage,
) {
    // Check if max retries exceeded
    if outboxMsg.Attempts >= p.config.MaxRetries {
        log.Printf("ERROR: Message %s exceeded max retries", outboxMsg.ID)
        p.store.MarkFailed(ctx, outboxMsg.ID,
            fmt.Errorf("exceeded max retries (%d)", p.config.MaxRetries))
        return
    }

    // Reconstruct message
    msg := message.New(outboxMsg.Payload, outboxMsg.Properties)

    // Publish to broker
    if err := p.publisher.Publish(ctx, msg); err != nil {
        log.Printf("ERROR: Failed to publish message %s: %v", outboxMsg.ID, err)

        // Record failure
        p.store.MarkFailed(ctx, outboxMsg.ID, err)

        // Apply backoff before next retry
        backoff := p.config.RetryBackoff(outboxMsg.Attempts)
        time.Sleep(backoff)
        return
    }

    // Mark as published
    if err := p.store.MarkPublished(ctx, outboxMsg.ID); err != nil {
        log.Printf("ERROR: Failed to mark message published: %v", err)
    }

    log.Printf("✅ Published message %s", outboxMsg.ID)
}

func (p *OutboxProcessor) cleanup(ctx context.Context) {
    if err := p.store.DeleteOld(ctx, p.config.CleanupAge); err != nil {
        log.Printf("ERROR: Failed to cleanup old messages: %v", err)
    }
}

// Stop gracefully stops the processor.
func (p *OutboxProcessor) Stop() {
    close(p.stopCh)
    <-p.stoppedCh
}

// BackoffFunc returns the backoff duration for a given attempt number.
type BackoffFunc func(attempts int) time.Duration

// ExponentialBackoff returns an exponential backoff function.
func ExponentialBackoff(base time.Duration, max time.Duration) BackoffFunc {
    return func(attempts int) time.Duration {
        backoff := base * time.Duration(math.Pow(2, float64(attempts)))
        if backoff > max {
            return max
        }
        return backoff
    }
}
```

## Usage Example

### 1. Setup

```go
// Setup database
db, _ := sql.Open("postgres", "postgres://localhost/mydb")

// Create outbox store
outboxStore := outbox.NewPostgresOutboxStore(db)

// Create publisher (your message broker)
publisher := &MyBrokerPublisher{client: natsClient}

// Create outbox processor
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

// Start background processor
go processor.Start(context.Background())

// Graceful shutdown
defer processor.Stop()
```

### 2. Usage in Command Handler

```go
// Command handler with transactional outbox
func handleCreateOrder(
    ctx context.Context,
    cmd CreateOrder,
    outboxStore outbox.OutboxStore,
    db *sql.DB,
) error {
    // Start transaction
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 1. Save order to database
    _, err = tx.ExecContext(ctx,
        `INSERT INTO orders (id, customer_id, amount, created_at)
         VALUES ($1, $2, $3, NOW())`,
        cmd.ID, cmd.CustomerID, cmd.Amount,
    )
    if err != nil {
        return err
    }

    // 2. Create event message
    evt := OrderCreated{
        ID:         cmd.ID,
        CustomerID: cmd.CustomerID,
        Amount:     cmd.Amount,
        CreatedAt:  time.Now(),
    }

    payload, _ := json.Marshal(evt)
    msg := message.New(payload, message.Properties{
        message.PropSubject:        "OrderCreated",
        message.PropCorrelationID:  cmd.CorrelationID,
        "type":                     "event",
    })

    // 3. Store message in outbox (within same transaction!)
    if err := outboxStore.Store(ctx, tx, msg); err != nil {
        return err
    }

    // 4. Commit transaction (atomic!)
    if err := tx.Commit(); err != nil {
        return err
    }

    // ✅ Both order and outbox message committed atomically
    // Background processor will publish the event
    return nil
}
```

### 3. Integration with CQRS Package

```go
// Create command handler with outbox support
createOrderHandler := cqrs.NewCommandHandlerWithOutbox(
    "CreateOrder",
    marshaler,
    outboxStore,
    db,
    func(ctx context.Context, cmd CreateOrder, tx Transaction) ([]OrderCreated, error) {
        // Save to database
        _, err := tx.(*sql.Tx).ExecContext(ctx,
            `INSERT INTO orders (id, customer_id, amount)
             VALUES ($1, $2, $3)`,
            cmd.ID, cmd.CustomerID, cmd.Amount,
        )
        if err != nil {
            return nil, err
        }

        // Return events (will be stored in outbox within same tx)
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    },
)
```

## Benefits

### 1. Exactly-Once Message Delivery

```
✅ Database committed + Message stored → Message will be published
✅ Database rolled back → Message not stored, won't be published
✅ Publish failed → Automatic retry
✅ Publish succeeded → Marked published, won't retry
```

### 2. Atomic Guarantees

```
┌─── Single Transaction ───┐
│ INSERT INTO orders       │ ← Both succeed or both fail
│ INSERT INTO outbox       │ ← Atomic!
│ COMMIT                   │
└──────────────────────────┘
```

### 3. Resilience

```
Scenario 1: Broker down
  → Messages accumulate in outbox
  → Automatic retry when broker recovers

Scenario 2: Application crashes
  → Outbox persisted in database
  → Processor resumes on restart

Scenario 3: Duplicate prevention
  → Published messages marked
  → Won't re-publish
```

## Implementation Phases

### Phase 1: Core Outbox (Priority: Low)

- [ ] `OutboxStore` interface
- [ ] `PostgresOutboxStore` implementation
- [ ] `OutboxProcessor` background worker
- [ ] Retry and backoff logic

### Phase 2: CQRS Integration (Priority: Low)

- [ ] `NewCommandHandlerWithOutbox` helper
- [ ] Transaction-aware handler wrapper
- [ ] Examples and documentation

### Phase 3: Advanced Features (Priority: Very Low)

- [ ] Metrics and monitoring
- [ ] Dead letter queue for failed messages
- [ ] Multi-database support (MySQL, etc.)
- [ ] Message ordering guarantees

## When to Use Transactional Outbox

### Use When:

✅ Need exactly-once message delivery
✅ Database writes + message publishing must be atomic
✅ Can't tolerate duplicate events
✅ Critical business operations (payments, orders)
✅ Have database transactions available

### Don't Use When:

❌ At-least-once delivery is acceptable
❌ Idempotent message processing handles duplicates
❌ No database (event-sourced systems)
❌ Performance overhead not acceptable
❌ Broker has native exactly-once support

## Performance Considerations

### Latency

- **Database write**: Same as normal (single transaction)
- **Event delivery**: Delayed by poll interval (default: 1s)
- **Tradeoff**: Consistency vs. low latency

**Optimization:**
```go
// Reduce poll interval for lower latency
config := OutboxConfig{
    PollInterval: 100 * time.Millisecond, // Lower latency
}
```

### Throughput

- **Database**: One additional INSERT per message
- **Processor**: Configurable batch size

**Optimization:**
```go
// Increase batch size for higher throughput
config := OutboxConfig{
    BatchSize: 1000, // Process more messages per batch
}
```

### Storage

- Outbox table grows over time
- Cleanup job removes old messages

**Optimization:**
```go
// More aggressive cleanup
config := OutboxConfig{
    CleanupInterval: 15 * time.Minute,
    CleanupAge:      1 * time.Hour,
}
```

## Alternatives

### Alternative 1: Change Data Capture (CDC)

Use database replication log (e.g., Debezium, Maxwell) to publish events.

**Pros:**
- ✅ No application code changes
- ✅ Very low overhead

**Cons:**
- ❌ Complex infrastructure
- ❌ Less control over event format
- ❌ Tight coupling to database schema

### Alternative 2: At-Least-Once + Idempotency

Accept duplicate messages, make consumers idempotent.

**Pros:**
- ✅ Simpler
- ✅ Better performance

**Cons:**
- ❌ Requires idempotent consumers
- ❌ Can't always make operations idempotent
- ❌ Duplicate processing overhead

### Alternative 3: Broker-Native Transactions

Some brokers support transactions (e.g., Kafka).

**Pros:**
- ✅ No outbox table needed
- ✅ Lower latency

**Cons:**
- ❌ Broker-specific
- ❌ Still dual writes (DB + broker)
- ❌ Complex coordination

## Migration Path

Existing handlers can be incrementally migrated:

**Step 1:** Set up outbox infrastructure
```go
outboxStore := outbox.NewPostgresOutboxStore(db)
processor := outbox.NewOutboxProcessor(...)
go processor.Start(ctx)
```

**Step 2:** Migrate critical handlers first
```go
// Migrate payment/order handlers to use outbox
createOrderHandler := cqrs.NewCommandHandlerWithOutbox(...)
```

**Step 3:** Keep non-critical handlers as-is
```go
// Analytics events can stay at-least-once
analyticsHandler := cqrs.NewEventHandler(...) // No outbox needed
```

## References

- [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)
- [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md)
- [Advanced Patterns Documentation](../cqrs-advanced-patterns.md)
- [Transactional Outbox Pattern - Microservices.io](https://microservices.io/patterns/data/transactional-outbox.html)
- [Debezium Outbox Pattern](https://debezium.io/documentation/reference/transformations/outbox-event-router.html)
- [Outbox Pattern - Chris Richardson](https://chrisrichardson.net/post/microservices/patterns/2020/04/27/transactional-outbox-polling.html)
