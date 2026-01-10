# Event Persistence

**Status:** Proposed

## Overview

Durable event storage with rich querying capabilities. The SQL Event Store enables event sourcing, audit logging, and transactional outbox patterns.

## Goals

1. Create `EventStore` interface for event persistence
2. Implement PostgreSQL and SQLite drivers
3. Support querying by CloudEvents attributes
4. Enable transactional outbox pattern
5. Provide event replay for rebuilding projections

## Core Interface

```go
// store/eventstore.go
type EventStore interface {
    // Append stores events atomically
    Append(ctx context.Context, events ...*message.Message) error

    // AppendTx stores events within an existing transaction
    AppendTx(ctx context.Context, tx Tx, events ...*message.Message) error

    // Query retrieves events matching criteria
    Query(ctx context.Context, query EventQuery) (EventIterator, error)

    // GetByID retrieves a specific event
    GetByID(ctx context.Context, source, id string) (*message.Message, error)

    // Stream returns events for a subject from sequence
    Stream(ctx context.Context, subject string, fromSeq int64) (EventIterator, error)

    // Close releases resources
    Close() error
}

type EventQuery struct {
    // CloudEvents filters
    Types         []string
    Sources       []string
    Subjects      []string
    CorrelationID string

    // Time range
    FromTime      time.Time
    ToTime        time.Time

    // Sequence range
    FromSequence  int64
    ToSequence    int64

    // Pagination
    Limit         int
    Offset        int64
    Order         QueryOrder
}

type EventIterator interface {
    Next() bool
    Event() *message.Message
    Sequence() int64
    Err() error
    Close() error
}
```

## PostgreSQL Schema

```sql
CREATE TABLE IF NOT EXISTS events (
    -- Internal fields
    sequence_id    BIGSERIAL PRIMARY KEY,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- CloudEvents required
    id             VARCHAR(255) NOT NULL,
    source         VARCHAR(2048) NOT NULL,
    specversion    VARCHAR(10) NOT NULL DEFAULT '1.0',
    type           VARCHAR(255) NOT NULL,

    -- CloudEvents optional
    subject        VARCHAR(2048),
    time           TIMESTAMPTZ,
    datacontenttype VARCHAR(255) DEFAULT 'application/json',

    -- gopipe extensions
    correlationid  VARCHAR(255),

    -- Event data
    data           JSONB,

    -- Constraints
    UNIQUE(source, id)
);

-- Indexes
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_source ON events(source);
CREATE INDEX idx_events_subject ON events(subject);
CREATE INDEX idx_events_time ON events(time);
CREATE INDEX idx_events_correlationid ON events(correlationid);
CREATE INDEX idx_events_stream ON events(subject, sequence_id);
```

## Transactional Outbox

```go
type TransactionalPublisher struct {
    db    *sql.DB
    store *SQLEventStore
}

func (p *TransactionalPublisher) PublishWithTx(
    ctx context.Context,
    businessLogic func(tx *sql.Tx) error,
    events ...*message.Message,
) error {
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Execute business logic
    if err := businessLogic(tx); err != nil {
        return err
    }

    // Append events in same transaction
    if err := p.store.AppendTx(ctx, tx, events...); err != nil {
        return err
    }

    return tx.Commit()
}

// Usage
pub.PublishWithTx(ctx, func(tx *sql.Tx) error {
    _, err := tx.Exec("UPDATE orders SET status = 'completed' WHERE id = $1", orderID)
    return err
}, orderCompletedEvent)
```

## Event Replay

```go
type Replayer struct {
    store EventStore
}

func (r *Replayer) ReplaySubject(
    ctx context.Context,
    subject string,
    handler func(ctx context.Context, msg *message.Message) error,
) error {
    iter, err := r.store.Stream(ctx, subject, 0)
    if err != nil {
        return err
    }
    defer iter.Close()

    for iter.Next() {
        if err := handler(ctx, iter.Event()); err != nil {
            return fmt.Errorf("replay seq %d: %w", iter.Sequence(), err)
        }
    }
    return iter.Err()
}
```

## Package Structure

```
store/
├── eventstore.go          # Interface definitions
├── query.go               # EventQuery, EventIterator
├── replay.go              # Replayer
├── outbox.go              # TransactionalPublisher
├── sql/
│   ├── store.go           # SQLEventStore
│   ├── iterator.go        # sqlEventIterator
│   ├── driver.go          # SQLDriver interface
│   ├── postgres/
│   │   ├── driver.go      # PostgresDriver
│   │   └── migrate.sql    # Schema
│   └── sqlite/
│       ├── driver.go      # SQLiteDriver
│       └── migrate.sql    # Schema
└── doc.go
```

## Usage Example

```go
func main() {
    // Setup database
    db, _ := sql.Open("postgres", "postgres://localhost/events")

    // Create event store
    store, _ := sqlstore.New(sqlstore.Config{
        DB:          db,
        Driver:      postgres.NewDriver(),
        AutoMigrate: true,
    })
    defer store.Close()

    // === Transactional Publishing ===
    pub := store.NewTransactionalPublisher(db)

    err := pub.PublishWithTx(ctx, func(tx *sql.Tx) error {
        _, err := tx.Exec("INSERT INTO orders ...")
        return err
    }, orderCreatedEvent)

    // === Querying ===
    iter, _ := store.Query(ctx, EventQuery{
        Types:    []string{"order.created"},
        FromTime: time.Now().Add(-24 * time.Hour),
        Limit:    100,
    })
    for iter.Next() {
        event := iter.Event()
        fmt.Printf("Event: %s at %v\n", event.Type(), iter.Sequence())
    }

    // === Replay ===
    replayer := NewReplayer(store)
    replayer.ReplaySubject(ctx, "order-123", func(ctx context.Context, msg *message.Message) error {
        // Rebuild state from events
        return nil
    })
}
```

## PR Sequence

| PR | Content |
|----|---------|
| 1 | EventStore interface + SQLDriver interface |
| 2 | PostgreSQL driver + SQLEventStore |
| 3 | SQLite driver |
| 4 | Transactional Outbox + Replay |
