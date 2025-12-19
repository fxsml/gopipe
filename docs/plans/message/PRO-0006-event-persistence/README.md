# PRO-0006: Event Persistence

**Status:** Proposed
**Priority:** Medium (can be done in parallel after Layer 1)
**Depends On:** PRO-0003-message-standardization
**Related ADRs:** 0025
**Related Features:** 15

## Overview

This extension provides durable event storage with rich querying capabilities. The SQL Event Store enables event sourcing, audit logging, and transactional outbox patterns.

## Goals

1. Create `EventStore` interface for event persistence
2. Implement PostgreSQL and SQLite drivers
3. Support querying by CloudEvents attributes
4. Enable transactional outbox pattern (ADR 0009)
5. Provide event replay for rebuilding projections

## Prerequisites

Layer 1 must be complete (for CloudEvents attributes):
- [ ] CloudEvents validation working
- [ ] Non-generic Message implemented

## Sub-Tasks

### Task E.1: EventStore Interface

**Implementation:**
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

type QueryOrder string

const (
    OrderAsc  QueryOrder = "ASC"
    OrderDesc QueryOrder = "DESC"
)

type EventIterator interface {
    Next() bool
    Event() *message.Message
    Sequence() int64
    Err() error
    Close() error
}
```

---

### Task E.2: SQL Driver Interface

**Implementation:**
```go
// store/sql/driver.go
type SQLDriver interface {
    Name() string
    MigrateSQL(tableName string) string
    InsertSQL(tableName string) string
    BuildQuery(tableName string, q EventQuery) (string, []any)
    Placeholder(n int) string
}

// Built-in drivers
func NewPostgresDriver() SQLDriver
func NewSQLiteDriver() SQLDriver
```

---

### Task E.3: PostgreSQL Schema

**Schema:**
```sql
-- store/sql/postgres/migrate.sql
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
    destination    VARCHAR(2048),
    topic          VARCHAR(255),
    correlationid  VARCHAR(255),

    -- Event data
    data           JSONB,
    data_binary    BYTEA,  -- For non-JSON content

    -- Extensions (arbitrary key-value)
    extensions     JSONB DEFAULT '{}',

    -- Constraints
    UNIQUE(source, id)
);

-- Indexes
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_source ON events(source);
CREATE INDEX idx_events_subject ON events(subject);
CREATE INDEX idx_events_time ON events(time);
CREATE INDEX idx_events_correlationid ON events(correlationid);
CREATE INDEX idx_events_topic ON events(topic);
CREATE INDEX idx_events_created_at ON events(created_at);
CREATE INDEX idx_events_stream ON events(subject, sequence_id);
```

---

### Task E.4: SQLEventStore Implementation

**Implementation:**
```go
// store/sql/store.go
type SQLEventStore struct {
    db         *sql.DB
    driver     SQLDriver
    serializer *message.Serializer
    tableName  string
}

type SQLEventStoreConfig struct {
    DB          *sql.DB
    Driver      SQLDriver
    Serializer  *message.Serializer
    TableName   string  // Default: "events"
    AutoMigrate bool
}

func NewSQLEventStore(config SQLEventStoreConfig) (*SQLEventStore, error)

func (s *SQLEventStore) Append(ctx context.Context, events ...*message.Message) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if err := s.AppendTx(ctx, tx, events...); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *SQLEventStore) AppendTx(ctx context.Context, tx Tx, events ...*message.Message) error {
    stmt, err := tx.PrepareContext(ctx, s.driver.InsertSQL(s.tableName))
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, event := range events {
        data, err := s.serializeData(event)
        if err != nil {
            return fmt.Errorf("serialize event %s: %w", event.ID(), err)
        }

        _, err = stmt.ExecContext(ctx,
            event.ID(),
            event.Source(),
            event.SpecVersion(),
            event.Type(),
            event.Subject(),
            event.Time(),
            event.DataContentType(),
            event.Destination(),
            event.Topic(),
            event.CorrelationID(),
            data,
            s.serializeExtensions(event),
        )
        if err != nil {
            return err
        }
    }
    return nil
}

func (s *SQLEventStore) Query(ctx context.Context, q EventQuery) (EventIterator, error) {
    query, args := s.driver.BuildQuery(s.tableName, q)
    rows, err := s.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    return &sqlEventIterator{rows: rows, store: s}, nil
}
```

---

### Task E.5: Transactional Outbox Support

**Implementation:**
```go
// store/outbox.go
type TransactionalPublisher struct {
    db    *sql.DB
    store *SQLEventStore
}

func NewTransactionalPublisher(db *sql.DB, store *SQLEventStore) *TransactionalPublisher

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

---

### Task E.6: Event Replay

**Implementation:**
```go
// store/replay.go
type Replayer struct {
    store EventStore
}

func NewReplayer(store EventStore) *Replayer

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

func (r *Replayer) ReplayByType(
    ctx context.Context,
    eventType string,
    fromTime time.Time,
    handler func(ctx context.Context, msg *message.Message) error,
) error {
    iter, err := r.store.Query(ctx, EventQuery{
        Types:    []string{eventType},
        FromTime: fromTime,
        Order:    OrderAsc,
    })
    if err != nil {
        return err
    }
    defer iter.Close()

    for iter.Next() {
        if err := handler(ctx, iter.Event()); err != nil {
            return err
        }
    }
    return iter.Err()
}
```

---

### Task E.7: Publisher Integration

**Implementation:**
```go
// store/publisher.go
type EventStoreSender struct {
    store EventStore
}

func NewEventStoreSender(store EventStore) *EventStoreSender

func (s *EventStoreSender) Send(ctx context.Context, msgs []*message.Message) error {
    return s.store.Append(ctx, msgs...)
}

// Usage with standard Publisher
store, _ := sql.NewSQLEventStore(config)
sender := store.NewEventStoreSender(store)
publisher := message.NewPublisher(sender, publisherConfig)
```

---

## Package Structure

```
store/
├── eventstore.go          # Interface definitions
├── query.go               # EventQuery, EventIterator
├── replay.go              # Replayer
├── outbox.go              # TransactionalPublisher
├── publisher.go           # EventStoreSender
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
    store, _ := sql.NewSQLEventStore(sql.SQLEventStoreConfig{
        DB:          db,
        Driver:      postgres.NewDriver(),
        AutoMigrate: true,
    })
    defer store.Close()

    // === Transactional Publishing ===
    pub := store.NewTransactionalPublisher(db, store)

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
    replayer := store.NewReplayer(store)
    replayer.ReplaySubject(ctx, "order-123", func(ctx context.Context, msg *message.Message) error {
        // Rebuild state from events
        return nil
    })
}
```

## Implementation Order

```
1. EventStore Interface ────────────────────┐
                                            │
2. SQLDriver Interface ─────────────────────┼──► 3. PostgreSQL Implementation
                                            │
                                            └──► 4. SQLite Implementation

5. Transactional Outbox ────────────────────────► Depends on 3 or 4

6. Event Replay ────────────────────────────────► Depends on 3 or 4

7. Publisher Integration ───────────────────────► Depends on 1
```

**Recommended PR Sequence:**
1. **PR 1:** EventStore interface + SQLDriver interface
2. **PR 2:** PostgreSQL driver + SQLEventStore
3. **PR 3:** SQLite driver
4. **PR 4:** Transactional Outbox + Replay + Publisher

## Validation Checklist

Before marking Extension complete:

- [ ] EventStore interface implemented
- [ ] PostgreSQL driver works
- [ ] SQLite driver works
- [ ] Auto-migration creates tables
- [ ] Query filters work (type, source, time, etc.)
- [ ] EventIterator streams results
- [ ] TransactionalPublisher commits atomically
- [ ] Replayer rebuilds state correctly
- [ ] EventStoreSender integrates with Publisher
- [ ] All tests pass
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0025: SQL Event Store](../adr/0025-sql-event-store.md)
- [ADR 0009: Transactional Outbox](../adr/0009-transactional-outbox-pattern.md)
- [Feature 15: SQL Event Store](../features/15-sql-event-store.md)
