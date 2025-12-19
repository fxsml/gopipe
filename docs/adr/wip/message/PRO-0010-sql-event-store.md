# ADR 0025: SQL Event Store

**Date:** 2025-12-13
**Status:** Proposed
**Depends on:** ADR 0019, ADR 0021, ADR 0024

## Context

To build complete event-sourced systems, gopipe needs an **event store** that:

1. Persists CloudEvents-compliant messages durably
2. Supports querying by CloudEvents attributes (type, source, subject, time)
3. Enables event replay for rebuilding state
4. Integrates with existing Sender interface
5. Supports transactions (for Transactional Outbox pattern - ADR 0009)

### Requirements

- **Durability**: Events must survive restarts
- **Queryability**: Filter by type, source, subject, time range, correlation ID
- **Ordering**: Maintain event order per stream/subject
- **Replay**: Support replaying events from any point
- **Transactions**: Atomic writes with business logic
- **Pluggable**: Support multiple databases (Postgres, SQLite, etc.)

### Alternatives Considered

1. **SQL Database** (Postgres, SQLite)
2. **NATS JetStream**
3. **Dedicated Event Store** (EventStoreDB)

## Decision

Implement a **SQL Event Store** with pluggable database drivers, starting with PostgreSQL.

### 1. Event Store Schema

```sql
-- events table stores all CloudEvents
CREATE TABLE IF NOT EXISTS events (
    -- Internal fields
    sequence_id    BIGSERIAL PRIMARY KEY,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- CloudEvents required attributes
    id             VARCHAR(255) NOT NULL,
    source         VARCHAR(2048) NOT NULL,
    specversion    VARCHAR(10) NOT NULL DEFAULT '1.0',
    type           VARCHAR(255) NOT NULL,

    -- CloudEvents optional attributes
    subject        VARCHAR(2048),
    time           TIMESTAMPTZ,
    datacontenttype VARCHAR(255) DEFAULT 'application/json',

    -- gopipe extensions
    destination    VARCHAR(2048),
    topic          VARCHAR(255),
    correlationid  VARCHAR(255),

    -- Event data
    data           JSONB,
    data_binary    BYTEA,  -- For non-JSON content types

    -- Extensions (arbitrary key-value)
    extensions     JSONB DEFAULT '{}',

    -- Constraints
    UNIQUE(source, id)  -- CloudEvents uniqueness
);

-- Indexes for common query patterns
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_source ON events(source);
CREATE INDEX idx_events_subject ON events(subject);
CREATE INDEX idx_events_time ON events(time);
CREATE INDEX idx_events_correlationid ON events(correlationid);
CREATE INDEX idx_events_topic ON events(topic);
CREATE INDEX idx_events_created_at ON events(created_at);

-- Composite index for stream queries
CREATE INDEX idx_events_stream ON events(subject, sequence_id);
```

### 2. Event Store Interface

```go
// EventStore provides event persistence and querying
type EventStore interface {
    // Append stores events atomically
    Append(ctx context.Context, events ...*Message) error

    // AppendTx stores events within an existing transaction
    AppendTx(ctx context.Context, tx Tx, events ...*Message) error

    // Query retrieves events matching criteria
    Query(ctx context.Context, query EventQuery) (EventIterator, error)

    // GetByID retrieves a specific event
    GetByID(ctx context.Context, source, id string) (*Message, error)

    // Stream returns events for a subject from sequence
    Stream(ctx context.Context, subject string, fromSeq int64) (EventIterator, error)

    // Close releases resources
    Close() error
}

// EventQuery defines query criteria
type EventQuery struct {
    // Filter by CloudEvents attributes
    Types         []string      // Event types to include
    Sources       []string      // Event sources to include
    Subjects      []string      // Subjects to include
    CorrelationID string        // Correlation ID filter

    // Time range
    FromTime      time.Time     // Events after this time
    ToTime        time.Time     // Events before this time

    // Sequence range
    FromSequence  int64         // Events after this sequence
    ToSequence    int64         // Events before this sequence

    // Pagination
    Limit         int           // Max events to return
    Offset        int64         // Skip first N events

    // Ordering
    Order         QueryOrder    // ASC or DESC by sequence
}

type QueryOrder string

const (
    OrderAsc  QueryOrder = "ASC"
    OrderDesc QueryOrder = "DESC"
)

// EventIterator allows streaming through query results
type EventIterator interface {
    // Next advances to next event, returns false when done
    Next() bool

    // Event returns current event
    Event() *Message

    // Sequence returns current event's sequence number
    Sequence() int64

    // Err returns any error encountered
    Err() error

    // Close releases iterator resources
    Close() error
}
```

### 3. SQL Implementation

```go
// SQLEventStore implements EventStore using SQL database
type SQLEventStore struct {
    db         *sql.DB
    driver     SQLDriver
    serializer *Serializer
    tableName  string
}

type SQLEventStoreConfig struct {
    DB         *sql.DB
    Driver     SQLDriver      // postgres, sqlite, mysql
    Serializer *Serializer
    TableName  string         // Default: "events"
    AutoMigrate bool          // Create table if not exists
}

func NewSQLEventStore(config SQLEventStoreConfig) (*SQLEventStore, error) {
    store := &SQLEventStore{
        db:         config.DB,
        driver:     config.Driver,
        serializer: config.Serializer,
        tableName:  config.TableName,
    }

    if config.AutoMigrate {
        if err := store.migrate(context.Background()); err != nil {
            return nil, fmt.Errorf("migrate: %w", err)
        }
    }

    return store, nil
}

func (s *SQLEventStore) Append(ctx context.Context, events ...*Message) error {
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

func (s *SQLEventStore) AppendTx(ctx context.Context, tx Tx, events ...*Message) error {
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
            return fmt.Errorf("insert event %s: %w", event.ID(), err)
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

### 4. Pluggable SQL Driver

```go
// SQLDriver abstracts database-specific SQL
type SQLDriver interface {
    // Name returns driver name (postgres, sqlite, mysql)
    Name() string

    // MigrateSQL returns CREATE TABLE statement
    MigrateSQL(tableName string) string

    // InsertSQL returns INSERT statement with placeholders
    InsertSQL(tableName string) string

    // BuildQuery builds SELECT with query criteria
    BuildQuery(tableName string, q EventQuery) (string, []any)

    // Placeholder returns parameter placeholder ($1, ?, :name)
    Placeholder(n int) string
}

// PostgresDriver implements SQLDriver for PostgreSQL
type PostgresDriver struct{}

func (d *PostgresDriver) Name() string { return "postgres" }

func (d *PostgresDriver) Placeholder(n int) string {
    return fmt.Sprintf("$%d", n)
}

func (d *PostgresDriver) InsertSQL(tableName string) string {
    return fmt.Sprintf(`
        INSERT INTO %s (id, source, specversion, type, subject, time,
                       datacontenttype, destination, topic, correlationid,
                       data, extensions)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    `, tableName)
}

// SQLiteDriver implements SQLDriver for SQLite
type SQLiteDriver struct{}

func (d *SQLiteDriver) Name() string { return "sqlite" }

func (d *SQLiteDriver) Placeholder(n int) string { return "?" }
```

### 5. Sender Implementation

```go
// EventStoreSender implements Sender interface for EventStore
type EventStoreSender struct {
    store EventStore
}

func NewEventStoreSender(store EventStore) *EventStoreSender {
    return &EventStoreSender{store: store}
}

func (s *EventStoreSender) Send(ctx context.Context, msgs []*Message) error {
    return s.store.Append(ctx, msgs...)
}

// Usage with Publisher
store, _ := NewSQLEventStore(config)
sender := NewEventStoreSender(store)
publisher := NewPublisher(sender, publisherConfig)
```

### 6. Transactional Outbox Support

Enables ADR 0009 (Transactional Outbox):

```go
// TransactionalPublisher combines business logic with event publishing
type TransactionalPublisher struct {
    db    *sql.DB
    store *SQLEventStore
}

func (p *TransactionalPublisher) PublishWithTx(
    ctx context.Context,
    businessLogic func(tx *sql.Tx) error,
    events ...*Message,
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
publisher.PublishWithTx(ctx, func(tx *sql.Tx) error {
    // Update order status
    _, err := tx.Exec("UPDATE orders SET status = 'completed' WHERE id = $1", orderID)
    return err
}, orderCompletedEvent)
```

### 7. Event Replay

```go
// Replayer rebuilds state by replaying events
type Replayer struct {
    store EventStore
}

func (r *Replayer) ReplaySubject(
    ctx context.Context,
    subject string,
    handler func(ctx context.Context, msg *Message) error,
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
    handler func(ctx context.Context, msg *Message) error,
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

## NATS JetStream Evaluation

### JetStream as Event Store

NATS JetStream can serve as an alternative event store:

```go
// JetStreamEventStore implements EventStore using NATS JetStream
type JetStreamEventStore struct {
    js         nats.JetStreamContext
    stream     string
    serializer *Serializer
}

func (s *JetStreamEventStore) Append(ctx context.Context, events ...*Message) error {
    for _, event := range events {
        subject := s.eventSubject(event)
        data, _ := s.serializer.Serialize(event)

        _, err := s.js.Publish(subject, data)
        if err != nil {
            return err
        }
    }
    return nil
}

func (s *JetStreamEventStore) Stream(ctx context.Context, subject string, fromSeq int64) (EventIterator, error) {
    sub, err := s.js.Subscribe(subject,
        nats.StartSequence(uint64(fromSeq)),
        nats.DeliverAll(),
    )
    if err != nil {
        return nil, err
    }
    return &jetStreamIterator{sub: sub, serializer: s.serializer}, nil
}
```

### Comparison: SQL vs NATS JetStream

| Feature | SQL (Postgres) | NATS JetStream |
|---------|----------------|----------------|
| **Durability** | ✅ ACID transactions | ✅ Persistence to disk |
| **Query by Type** | ✅ Index scan | ⚠️ Subject wildcards only |
| **Query by Time** | ✅ Efficient range queries | ⚠️ StartTime consumer option |
| **Query by Arbitrary Attributes** | ✅ Full WHERE clause | ❌ Not supported |
| **Correlation ID Queries** | ✅ Indexed lookup | ❌ Not indexed |
| **Transaction Support** | ✅ Full ACID | ❌ No multi-message tx |
| **Ordering** | ✅ Per-stream guaranteed | ✅ Per-subject guaranteed |
| **Replay** | ✅ From any point | ✅ From sequence/time |
| **Scale** | ⚠️ Single DB (replication needed) | ✅ Built-in clustering |
| **Dependencies** | Database server | NATS server |
| **Embedded Option** | SQLite | Embedded NATS |

### Recommendation

| Use Case | Recommended |
|----------|-------------|
| Complex queries (multi-attribute) | **SQL** |
| Transactional outbox | **SQL** |
| High-throughput streaming | **JetStream** |
| Simple replay by subject | Either |
| Audit log with search | **SQL** |
| Distributed systems | **JetStream** |
| Zero infrastructure | SQLite or Embedded NATS |

### Hybrid Approach

For best of both worlds, use both:

```go
// Write to JetStream for real-time, project to SQL for queries
type HybridEventStore struct {
    primary   *JetStreamEventStore  // Real-time streaming
    secondary *SQLEventStore        // Query projection
}

func (h *HybridEventStore) Append(ctx context.Context, events ...*Message) error {
    // Write to JetStream (primary)
    if err := h.primary.Append(ctx, events...); err != nil {
        return err
    }

    // Async projection to SQL (eventual consistency)
    go h.secondary.Append(context.Background(), events...)

    return nil
}
```

## Rationale

1. **SQL First**: Most applications need complex queries
2. **Pluggable Drivers**: Support Postgres (production) and SQLite (testing/embedded)
3. **Sender Interface**: Seamless integration with Publisher
4. **Transaction Support**: Enables Transactional Outbox pattern
5. **JetStream Optional**: Available via gopipe-nats for streaming-first use cases

## Consequences

### Positive

- Durable event persistence
- Rich querying by any CloudEvents attribute
- Transaction support for consistency
- Event replay for rebuilding state
- Works with existing Publisher/Sender patterns

### Negative

- Database dependency (can use SQLite for zero-dep)
- Schema migrations needed
- Query performance depends on indexing

## Links

- [ADR 0009: Transactional Outbox Pattern](0009-transactional-outbox-pattern.md)
- [ADR 0019: CloudEvents Mandatory](0019-cloudevents-mandatory.md)
- [ADR 0021: ContentType Serialization](0021-contenttype-serialization.md)
- [Feature 15: SQL Event Store](../features/15-sql-event-store.md)
- [NATS JetStream Documentation](https://docs.nats.io/nats-concepts/jetstream)
