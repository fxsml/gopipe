# Feature: SQL Event Store

**Package:** `message/store`
**Status:** Proposed
**Related ADRs:**
- [ADR 0025](../adr/0025-sql-event-store.md) - SQL Event Store
- [ADR 0009](../adr/0009-transactional-outbox-pattern.md) - Transactional Outbox Pattern

## Summary

SQL-based event store for persisting and querying CloudEvents-compliant messages. Supports pluggable database drivers (PostgreSQL, SQLite), rich querying, event replay, and transactional writes for the Outbox pattern.

## Motivation

- Durable event persistence for event sourcing
- Rich querying by CloudEvents attributes
- Transaction support for consistency guarantees
- Event replay for rebuilding projections
- Audit trail with full search capability

## Implementation

### Package Structure

```
message/store/
├── store.go           # EventStore interface
├── query.go           # EventQuery and QueryBuilder
├── iterator.go        # EventIterator interface
├── sql_store.go       # SQL implementation
├── sql_iterator.go    # SQL iterator
├── driver.go          # SQLDriver interface
├── postgres.go        # PostgreSQL driver
├── sqlite.go          # SQLite driver
├── sender.go          # Sender adapter
├── replayer.go        # Event replay utilities
└── store_test.go      # Tests
```

### EventStore Interface

```go
package store

// EventStore provides event persistence and querying
type EventStore interface {
    // Append stores events atomically
    Append(ctx context.Context, events ...*message.Message) error

    // AppendTx stores events within an existing transaction
    AppendTx(ctx context.Context, tx Tx, events ...*message.Message) error

    // Query retrieves events matching criteria
    Query(ctx context.Context, query EventQuery) (EventIterator, error)

    // GetByID retrieves a specific event by source and id
    GetByID(ctx context.Context, source, id string) (*message.Message, error)

    // Stream returns events for a subject from sequence
    Stream(ctx context.Context, subject string, fromSeq int64) (EventIterator, error)

    // LastSequence returns the last sequence number
    LastSequence(ctx context.Context) (int64, error)

    // Close releases resources
    Close() error
}

// Tx abstracts database transaction
type Tx interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}
```

### EventQuery

```go
// EventQuery defines query criteria
type EventQuery struct {
    // CloudEvents attribute filters
    Types         []string      // Filter by event type
    Sources       []string      // Filter by source
    Subjects      []string      // Filter by subject
    CorrelationID string        // Filter by correlation ID

    // Time filters
    FromTime      time.Time     // Events after this time
    ToTime        time.Time     // Events before this time

    // Sequence filters
    FromSequence  int64         // Events after this sequence
    ToSequence    int64         // Events before this sequence

    // Destination filter (for internal routing queries)
    Destinations  []string      // Filter by destination

    // Pagination
    Limit         int           // Max events to return (0 = unlimited)
    Offset        int64         // Skip first N events

    // Ordering
    Order         QueryOrder    // ASC or DESC by sequence
}

// QueryBuilder provides fluent query construction
type QueryBuilder struct {
    query EventQuery
}

func NewQuery() *QueryBuilder {
    return &QueryBuilder{query: EventQuery{Order: OrderAsc}}
}

func (b *QueryBuilder) Types(types ...string) *QueryBuilder {
    b.query.Types = types
    return b
}

func (b *QueryBuilder) Sources(sources ...string) *QueryBuilder {
    b.query.Sources = sources
    return b
}

func (b *QueryBuilder) Subject(subject string) *QueryBuilder {
    b.query.Subjects = []string{subject}
    return b
}

func (b *QueryBuilder) CorrelationID(id string) *QueryBuilder {
    b.query.CorrelationID = id
    return b
}

func (b *QueryBuilder) Since(t time.Time) *QueryBuilder {
    b.query.FromTime = t
    return b
}

func (b *QueryBuilder) Until(t time.Time) *QueryBuilder {
    b.query.ToTime = t
    return b
}

func (b *QueryBuilder) FromSequence(seq int64) *QueryBuilder {
    b.query.FromSequence = seq
    return b
}

func (b *QueryBuilder) Limit(n int) *QueryBuilder {
    b.query.Limit = n
    return b
}

func (b *QueryBuilder) Desc() *QueryBuilder {
    b.query.Order = OrderDesc
    return b
}

func (b *QueryBuilder) Build() EventQuery {
    return b.query
}
```

### EventIterator

```go
// EventIterator streams query results
type EventIterator interface {
    // Next advances to next event
    Next() bool

    // Event returns current event
    Event() *message.Message

    // Sequence returns current sequence number
    Sequence() int64

    // Err returns any error
    Err() error

    // Close releases resources
    Close() error
}

// CollectAll reads all events from iterator
func CollectAll(iter EventIterator) ([]*message.Message, error) {
    defer iter.Close()

    var events []*message.Message
    for iter.Next() {
        events = append(events, iter.Event())
    }
    return events, iter.Err()
}

// ForEach iterates with callback
func ForEach(iter EventIterator, fn func(*message.Message, int64) error) error {
    defer iter.Close()

    for iter.Next() {
        if err := fn(iter.Event(), iter.Sequence()); err != nil {
            return err
        }
    }
    return iter.Err()
}
```

### SQL Event Store Implementation

```go
// SQLEventStore implements EventStore using SQL
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
    AutoMigrate bool    // Create table on startup
}

func NewSQLEventStore(config SQLEventStoreConfig) (*SQLEventStore, error) {
    if config.TableName == "" {
        config.TableName = "events"
    }
    if config.Driver == nil {
        config.Driver = &PostgresDriver{}
    }

    store := &SQLEventStore{
        db:         config.DB,
        driver:     config.Driver,
        serializer: config.Serializer,
        tableName:  config.TableName,
    }

    if config.AutoMigrate {
        if err := store.Migrate(context.Background()); err != nil {
            return nil, fmt.Errorf("migrate: %w", err)
        }
    }

    return store, nil
}

func (s *SQLEventStore) Migrate(ctx context.Context) error {
    _, err := s.db.ExecContext(ctx, s.driver.MigrateSQL(s.tableName))
    return err
}

func (s *SQLEventStore) Append(ctx context.Context, events ...*message.Message) error {
    if len(events) == 0 {
        return nil
    }

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
        return fmt.Errorf("prepare: %w", err)
    }
    defer stmt.Close()

    for _, event := range events {
        if err := s.insertEvent(ctx, stmt, event); err != nil {
            return err
        }
    }

    return nil
}

func (s *SQLEventStore) insertEvent(ctx context.Context, stmt *sql.Stmt, event *message.Message) error {
    // Serialize data based on content type
    var dataJSON []byte
    var dataBinary []byte

    ct := event.DataContentType()
    if ct == "" || strings.Contains(ct, "json") {
        data, err := json.Marshal(event.Data)
        if err != nil {
            return fmt.Errorf("marshal data: %w", err)
        }
        dataJSON = data
    } else {
        if b, ok := event.Data.([]byte); ok {
            dataBinary = b
        }
    }

    // Serialize extensions
    extensions, _ := json.Marshal(event.Extensions())

    _, err := stmt.ExecContext(ctx,
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
        dataJSON,
        dataBinary,
        extensions,
    )
    if err != nil {
        return fmt.Errorf("insert event %s: %w", event.ID(), err)
    }

    return nil
}

func (s *SQLEventStore) Query(ctx context.Context, q EventQuery) (EventIterator, error) {
    sql, args := s.driver.BuildSelectSQL(s.tableName, q)
    rows, err := s.db.QueryContext(ctx, sql, args...)
    if err != nil {
        return nil, err
    }
    return &sqlIterator{rows: rows, store: s}, nil
}
```

### Pluggable SQL Drivers

```go
// SQLDriver abstracts database-specific SQL
type SQLDriver interface {
    Name() string
    MigrateSQL(tableName string) string
    InsertSQL(tableName string) string
    BuildSelectSQL(tableName string, q EventQuery) (string, []any)
    Placeholder(n int) string
}

// PostgresDriver for PostgreSQL
type PostgresDriver struct{}

func (d *PostgresDriver) Name() string { return "postgres" }

func (d *PostgresDriver) Placeholder(n int) string {
    return fmt.Sprintf("$%d", n)
}

func (d *PostgresDriver) MigrateSQL(tableName string) string {
    return fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            sequence_id     BIGSERIAL PRIMARY KEY,
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            id              VARCHAR(255) NOT NULL,
            source          VARCHAR(2048) NOT NULL,
            specversion     VARCHAR(10) NOT NULL DEFAULT '1.0',
            type            VARCHAR(255) NOT NULL,
            subject         VARCHAR(2048),
            time            TIMESTAMPTZ,
            datacontenttype VARCHAR(255) DEFAULT 'application/json',
            destination     VARCHAR(2048),
            topic           VARCHAR(255),
            correlationid   VARCHAR(255),
            data            JSONB,
            data_binary     BYTEA,
            extensions      JSONB DEFAULT '{}',
            UNIQUE(source, id)
        );

        CREATE INDEX IF NOT EXISTS idx_%s_type ON %s(type);
        CREATE INDEX IF NOT EXISTS idx_%s_source ON %s(source);
        CREATE INDEX IF NOT EXISTS idx_%s_subject ON %s(subject);
        CREATE INDEX IF NOT EXISTS idx_%s_time ON %s(time);
        CREATE INDEX IF NOT EXISTS idx_%s_correlationid ON %s(correlationid);
        CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at);
    `, tableName,
        tableName, tableName,
        tableName, tableName,
        tableName, tableName,
        tableName, tableName,
        tableName, tableName,
        tableName, tableName)
}

// SQLiteDriver for SQLite
type SQLiteDriver struct{}

func (d *SQLiteDriver) Name() string { return "sqlite" }

func (d *SQLiteDriver) Placeholder(n int) string { return "?" }

func (d *SQLiteDriver) MigrateSQL(tableName string) string {
    return fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            sequence_id     INTEGER PRIMARY KEY AUTOINCREMENT,
            created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            id              TEXT NOT NULL,
            source          TEXT NOT NULL,
            specversion     TEXT NOT NULL DEFAULT '1.0',
            type            TEXT NOT NULL,
            subject         TEXT,
            time            DATETIME,
            datacontenttype TEXT DEFAULT 'application/json',
            destination     TEXT,
            topic           TEXT,
            correlationid   TEXT,
            data            TEXT,
            data_binary     BLOB,
            extensions      TEXT DEFAULT '{}',
            UNIQUE(source, id)
        );

        CREATE INDEX IF NOT EXISTS idx_%s_type ON %s(type);
        CREATE INDEX IF NOT EXISTS idx_%s_subject ON %s(subject);
    `, tableName, tableName, tableName, tableName, tableName)
}
```

### Sender Adapter

```go
// EventStoreSender adapts EventStore to Sender interface
type EventStoreSender struct {
    store EventStore
}

func NewEventStoreSender(store EventStore) *EventStoreSender {
    return &EventStoreSender{store: store}
}

func (s *EventStoreSender) Send(ctx context.Context, msgs []*message.Message) error {
    return s.store.Append(ctx, msgs...)
}

// Integrate with Publisher
func Example() {
    store, _ := NewSQLEventStore(config)
    sender := NewEventStoreSender(store)

    publisher := message.NewPublisher(sender, message.PublisherConfig{
        MaxBatchSize: 100,
        MaxDuration:  time.Second,
    })
}
```

### Event Replay

```go
// Replayer provides event replay utilities
type Replayer struct {
    store EventStore
}

func NewReplayer(store EventStore) *Replayer {
    return &Replayer{store: store}
}

// ReplaySubject replays all events for a subject
func (r *Replayer) ReplaySubject(
    ctx context.Context,
    subject string,
    handler func(ctx context.Context, msg *message.Message, seq int64) error,
) error {
    iter, err := r.store.Stream(ctx, subject, 0)
    if err != nil {
        return err
    }
    return ForEach(iter, func(msg *message.Message, seq int64) error {
        return handler(ctx, msg, seq)
    })
}

// ReplayByType replays events of specific type
func (r *Replayer) ReplayByType(
    ctx context.Context,
    eventType string,
    fromTime time.Time,
    handler func(ctx context.Context, msg *message.Message) error,
) error {
    query := NewQuery().
        Types(eventType).
        Since(fromTime).
        Build()

    iter, err := r.store.Query(ctx, query)
    if err != nil {
        return err
    }
    return ForEach(iter, func(msg *message.Message, _ int64) error {
        return handler(ctx, msg)
    })
}

// ReplayCorrelation replays all events in a correlation chain
func (r *Replayer) ReplayCorrelation(
    ctx context.Context,
    correlationID string,
    handler func(ctx context.Context, msg *message.Message) error,
) error {
    query := NewQuery().
        CorrelationID(correlationID).
        Build()

    iter, err := r.store.Query(ctx, query)
    if err != nil {
        return err
    }
    return ForEach(iter, func(msg *message.Message, _ int64) error {
        return handler(ctx, msg)
    })
}
```

### Transactional Outbox Integration

```go
// TransactionalEventStore combines business logic with event publishing
type TransactionalEventStore struct {
    db    *sql.DB
    store *SQLEventStore
}

func NewTransactionalEventStore(db *sql.DB, store *SQLEventStore) *TransactionalEventStore {
    return &TransactionalEventStore{db: db, store: store}
}

// ExecuteWithEvents runs business logic and publishes events atomically
func (t *TransactionalEventStore) ExecuteWithEvents(
    ctx context.Context,
    businessLogic func(tx *sql.Tx) error,
    events ...*message.Message,
) error {
    tx, err := t.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Execute business logic
    if err := businessLogic(tx); err != nil {
        return fmt.Errorf("business logic: %w", err)
    }

    // Append events atomically
    if err := t.store.AppendTx(ctx, tx, events...); err != nil {
        return fmt.Errorf("append events: %w", err)
    }

    return tx.Commit()
}
```

## Usage Examples

### Basic Usage

```go
// Setup
db, _ := sql.Open("postgres", "postgres://localhost/events")
store, _ := store.NewSQLEventStore(store.SQLEventStoreConfig{
    DB:          db,
    Driver:      &store.PostgresDriver{},
    AutoMigrate: true,
})
defer store.Close()

// Append events
event := message.MustNew(OrderCreated{ID: "123"}, message.Attributes{
    message.AttrID:          uuid.NewString(),
    message.AttrSource:      "/orders",
    message.AttrSpecVersion: "1.0",
    message.AttrType:        "order.created",
    message.AttrSubject:     "orders/123",
})
store.Append(ctx, event)

// Query events
query := store.NewQuery().
    Types("order.created", "order.completed").
    Since(time.Now().Add(-24 * time.Hour)).
    Limit(100).
    Build()

iter, _ := store.Query(ctx, query)
events, _ := store.CollectAll(iter)
```

### With Publisher

```go
// Create event store sender
sender := store.NewEventStoreSender(eventStore)

// Use with standard Publisher
publisher := message.NewPublisher(sender, message.PublisherConfig{
    MaxBatchSize: 50,
})

// Publish through normal flow
router.Output(publisher.Input())
```

### Transactional Outbox

```go
txStore := store.NewTransactionalEventStore(db, eventStore)

// Business logic + events in one transaction
err := txStore.ExecuteWithEvents(ctx,
    func(tx *sql.Tx) error {
        _, err := tx.Exec("UPDATE orders SET status = $1 WHERE id = $2",
            "completed", orderID)
        return err
    },
    orderCompletedEvent,
    notificationEvent,
)
```

### Event Replay

```go
replayer := store.NewReplayer(eventStore)

// Rebuild order projection
replayer.ReplaySubject(ctx, "orders/123",
    func(ctx context.Context, msg *message.Message, seq int64) error {
        // Apply event to projection
        return projection.Apply(msg)
    })

// Replay all orders since yesterday
replayer.ReplayByType(ctx, "order.created", yesterday,
    func(ctx context.Context, msg *message.Message) error {
        return handleOrderCreated(ctx, msg)
    })
```

## NATS JetStream Alternative

See [Feature 14: NATS Integration](14-nats-integration.md) for JetStream-based event store.

### When to Use Which

| Scenario | SQL | JetStream |
|----------|-----|-----------|
| Need complex queries | ✅ | ❌ |
| Need ACID transactions | ✅ | ❌ |
| High-throughput streaming | ⚠️ | ✅ |
| Distributed deployment | ⚠️ | ✅ |
| Zero dependencies | SQLite | Embedded NATS |
| Audit/compliance queries | ✅ | ❌ |

## Files

- `message/store/store.go` - EventStore interface
- `message/store/query.go` - Query types and builder
- `message/store/iterator.go` - Iterator interface
- `message/store/sql_store.go` - SQL implementation
- `message/store/postgres.go` - PostgreSQL driver
- `message/store/sqlite.go` - SQLite driver
- `message/store/sender.go` - Sender adapter
- `message/store/replayer.go` - Replay utilities
- `message/store/transactional.go` - Outbox support

## Testing

```go
func TestSQLEventStore_AppendAndQuery(t *testing.T) {
    // Use SQLite for testing
    db, _ := sql.Open("sqlite3", ":memory:")
    store, _ := NewSQLEventStore(SQLEventStoreConfig{
        DB:          db,
        Driver:      &SQLiteDriver{},
        AutoMigrate: true,
    })

    // Append
    event := message.MustNew("data", message.Attributes{
        message.AttrID:          "1",
        message.AttrSource:      "/test",
        message.AttrSpecVersion: "1.0",
        message.AttrType:        "test.event",
    })
    require.NoError(t, store.Append(ctx, event))

    // Query
    query := NewQuery().Types("test.event").Build()
    iter, _ := store.Query(ctx, query)
    events, _ := CollectAll(iter)

    assert.Len(t, events, 1)
    assert.Equal(t, "1", events[0].ID())
}
```

## Related Features

- [09-cloudevents-mandatory](09-cloudevents-mandatory.md) - CloudEvents attributes
- [11-contenttype-serialization](11-contenttype-serialization.md) - Data serialization
- [13-internal-message-loop](13-internal-message-loop.md) - Integration with internal routing
- [14-nats-integration](14-nats-integration.md) - JetStream alternative
