# Plan: Inbox/Outbox Pattern

**Status:** Proposed
**Depends On:** [transaction-handling](transaction-handling.md)

## Overview

Add inbox (idempotent consumer) and outbox (transactional event publishing) middlewares that compose with TxMiddleware. All three share the same database transaction, giving exactly-once processing semantics: dedup check + business state + outbox entries commit atomically.

A polling-based outbox publisher reads from the outbox table and publishes to the real broker.

## Goals

1. Exactly-once processing via inbox + outbox in same TX
2. Composable middlewares: TX alone, TX + inbox, TX + inbox + outbox
3. Reliable event publishing via polling outbox publisher
4. Explicit schema management (no auto-migration in constructors)
5. Configurable cleanup for both inbox and outbox tables

## Architecture

### Middleware Stack

```
Router [Acking → TxMiddleware → InboxMiddleware → OutboxMiddleware → Handler]
          ↑           ↑               ↑                  ↑               ↑
     ack/nack     BEGIN TX      INSERT msg_id      intercept outputs  business logic
                  COMMIT        (dedup check)      (write to outbox)
```

All three middlewares share the TX via `TxFromContext(ctx)`. Middleware order matters:
- **TxMiddleware** (outermost user middleware): creates the TX
- **InboxMiddleware**: dedup check as first operation in TX
- **OutboxMiddleware**: intercept outputs as last operation in TX

### Outbox Publisher

```
OutboxPublisher (polls DB) ──▶ BrokerPublisher
         │                           │
    SELECT WHERE                on ack: UPDATE
    published = false           published = true
```

The publisher polls the outbox table, emits messages with acking that marks rows as published on ack. If publishing fails (nack), the row stays unpublished for the next poll.

## Tasks

### Task 1: Schema Management

**Goal:** Provide table schemas and explicit `Init` method for creating tables. Not in the constructor — Go constructors should not do I/O.

**Design:** Each middleware has an `Init(ctx context.Context) error` method that runs `CREATE TABLE IF NOT EXISTS`. Users can call it at startup or manage schema themselves via their own migration tool.

**Implementation:**
```go
// message/sql/schema.go

// Inbox table schema
const InboxSchema = `CREATE TABLE IF NOT EXISTS %s (
    subscriber  TEXT NOT NULL,
    message_id  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subscriber, message_id)
)`

// Outbox table schema
const OutboxSchema = `CREATE TABLE IF NOT EXISTS %s (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    topic       TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    data        JSONB NOT NULL,
    attributes  JSONB NOT NULL,
    published   BOOLEAN NOT NULL DEFAULT FALSE
)`
```

```go
// Init creates the table if it doesn't exist.
func (m *InboxMiddleware) Init(ctx context.Context) error {
    _, err := m.db.ExecContext(ctx, fmt.Sprintf(InboxSchema, m.table))
    return err
}
```

**Rationale:** Watermill uses lazy auto-create on first `Publish()` with advisory locks. River uses explicit migration APIs. For gopipe, explicit `Init` is simpler and safer — no DDL in the hot path, no advisory locks, no surprises inside user transactions.

**Files to Create:**
- `message/sql/schema.go` — table DDL constants, Init helpers

**Acceptance Criteria:**
- [ ] Inbox and outbox schemas defined as constants
- [ ] `Init(ctx)` runs `CREATE TABLE IF NOT EXISTS`
- [ ] Idempotent (safe to call multiple times)
- [ ] Table names configurable

### Task 2: InboxMiddleware (Idempotent Consumer)

**Goal:** Dedup incoming messages via a `processed_messages` table within the same TX as business logic.

**Design:** INSERT the message ID into the inbox table. If it's a duplicate (unique constraint violation), skip the handler. If it's new, proceed. The INSERT and the handler's DB writes share the same TX — atomic dedup.

**Implementation:**
```go
// message/sql/inbox_middleware.go

type InboxConfig struct {
    DB         *sql.DB
    Table      string        // default: "processed_messages"
    Subscriber string        // identifies this consumer (for multi-consumer dedup)
}

func InboxMiddleware(cfg InboxConfig) message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            tx, ok := TxFromContext(msg.Context(ctx))
            if !ok {
                return nil, fmt.Errorf("inbox: no transaction in context (TxMiddleware required)")
            }

            msgID := msg.ID()
            if msgID == "" {
                return nil, fmt.Errorf("inbox: message has no ID attribute")
            }

            _, err := tx.ExecContext(ctx,
                fmt.Sprintf("INSERT INTO %s (subscriber, message_id) VALUES ($1, $2)", cfg.Table),
                cfg.Subscriber, msgID,
            )
            if isDuplicateKeyError(err) {
                return nil, nil // already processed, skip
            }
            if err != nil {
                return nil, fmt.Errorf("inbox: %w", err)
            }

            return next(ctx, msg)
        }
    }
}
```

```go
// isDuplicateKeyError detects PostgreSQL unique constraint violations.
func isDuplicateKeyError(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

**Key design:** The inbox returns `nil, nil` for duplicates (no outputs, no error). With acking outermost, this causes `AckOnSuccess` to ack the message — the correct behavior for a duplicate that was already processed.

**Files to Create:**
- `message/sql/inbox_middleware.go`
- `message/sql/inbox_middleware_test.go`

**Acceptance Criteria:**
- [ ] New messages pass through to handler
- [ ] Duplicate messages (same ID + subscriber) are skipped silently
- [ ] Dedup check runs inside the TX from TxMiddleware
- [ ] Requires TxMiddleware (returns error if no TX in context)
- [ ] Returns `nil, nil` for duplicates (acking middleware acks)

### Task 3: OutboxMiddleware (Transactional Event Capture)

**Goal:** Intercept output messages from the handler and write them to the outbox table within the same TX. No output events are passed downstream — the outbox publisher handles delivery.

**Implementation:**
```go
// message/sql/outbox_middleware.go

type OutboxConfig struct {
    DB    *sql.DB
    Table string  // default: "outbox"
    Topic string  // target topic for the outbox publisher
}

func OutboxMiddleware(cfg OutboxConfig) message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            outputs, err := next(ctx, msg)
            if err != nil {
                return nil, err
            }

            if len(outputs) == 0 {
                return nil, nil
            }

            tx, ok := TxFromContext(msg.Context(ctx))
            if !ok {
                return nil, fmt.Errorf("outbox: no transaction in context (TxMiddleware required)")
            }

            for _, out := range outputs {
                data, err := json.Marshal(out.Data)
                if err != nil {
                    return nil, fmt.Errorf("outbox: marshal data: %w", err)
                }
                attrs, err := json.Marshal(out.Attributes)
                if err != nil {
                    return nil, fmt.Errorf("outbox: marshal attributes: %w", err)
                }

                _, err = tx.ExecContext(ctx,
                    fmt.Sprintf(
                        "INSERT INTO %s (topic, event_type, data, attributes) VALUES ($1, $2, $3, $4)",
                        cfg.Table,
                    ),
                    cfg.Topic, out.Type(), data, attrs,
                )
                if err != nil {
                    return nil, fmt.Errorf("outbox: insert: %w", err)
                }
            }

            return nil, nil // swallow outputs — outbox publisher handles delivery
        }
    }
}
```

**Key design:** The handler still returns output events normally — its contract is unchanged. The outbox middleware intercepts them transparently, serializes to the outbox table, and returns no outputs. TxMiddleware commits everything (business state + outbox entries) atomically.

**Files to Create:**
- `message/sql/outbox_middleware.go`
- `message/sql/outbox_middleware_test.go`

**Acceptance Criteria:**
- [ ] Output messages serialized to outbox table within TX
- [ ] No output messages passed downstream (returns `nil, nil`)
- [ ] Handler contract unchanged (still returns `[]*Message`)
- [ ] Requires TxMiddleware (returns error if no TX in context)
- [ ] Data and Attributes serialized as JSON

### Task 4: OutboxPublisher (Polling)

**Goal:** Poll the outbox table and publish messages to the real broker. Ack marks rows as published.

**Implementation:**
```go
// message/sql/outbox_publisher.go

type OutboxPublisherConfig struct {
    DB            *sql.DB
    Table         string        // default: "outbox"
    PollInterval  time.Duration // default: 500ms
    BatchSize     int           // default: 100
}

type OutboxPublisher struct {
    cfg OutboxPublisherConfig
}

func NewOutboxPublisher(cfg OutboxPublisherConfig) *OutboxPublisher {
    // apply defaults
    return &OutboxPublisher{cfg: cfg}
}

// Source returns a channel of messages read from the outbox table.
// Each message's acking marks the row as published on ack.
// Runs until ctx is cancelled.
func (p *OutboxPublisher) Source(ctx context.Context) (<-chan *message.RawMessage, error) {
    out := make(chan *message.RawMessage, p.cfg.BatchSize)
    go func() {
        defer close(out)
        ticker := time.NewTicker(p.cfg.PollInterval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                p.poll(ctx, out)
            }
        }
    }()
    return out, nil
}

func (p *OutboxPublisher) poll(ctx context.Context, out chan<- *message.RawMessage) {
    rows, err := p.cfg.DB.QueryContext(ctx,
        fmt.Sprintf(
            "SELECT id, data, attributes FROM %s WHERE published = false ORDER BY id LIMIT $1",
            p.cfg.Table,
        ),
        p.cfg.BatchSize,
    )
    if err != nil {
        return // log, retry on next tick
    }
    defer rows.Close()

    for rows.Next() {
        var id int64
        var data, attrs []byte
        if err := rows.Scan(&id, &data, &attrs); err != nil {
            continue
        }

        // Acking marks row as published
        acking := message.NewAcking(
            func() {
                p.cfg.DB.ExecContext(ctx,
                    fmt.Sprintf("UPDATE %s SET published = true WHERE id = $1", p.cfg.Table),
                    id,
                )
            },
            func(e error) { /* leave for next poll */ },
        )

        var msgAttrs message.Attributes
        json.Unmarshal(attrs, &msgAttrs)

        msg := message.NewRaw(data, msgAttrs, acking)

        select {
        case out <- msg:
        case <-ctx.Done():
            return
        }
    }
}
```

**Key design:** The publisher emits `RawMessage` (serialized data). Ack = mark published, Nack = leave for retry on next poll. If the process crashes between publish and mark, the next poll picks it up — at-least-once delivery from outbox to broker.

**Files to Create:**
- `message/sql/outbox_publisher.go`
- `message/sql/outbox_publisher_test.go`

**Acceptance Criteria:**
- [ ] Polls outbox table at configurable interval
- [ ] Emits RawMessage with acking bound to row state
- [ ] Ack marks row as published
- [ ] Nack leaves row unpublished (retried on next poll)
- [ ] Respects context cancellation for clean shutdown
- [ ] Batch size configurable

### Task 5: Cleanup

**Goal:** Remove old inbox and outbox entries to prevent unbounded table growth.

**Design:** Background goroutine with configurable retention period. Runs DELETE periodically. Respects context cancellation for clean shutdown.

**Implementation:**
```go
// message/sql/cleanup.go

type CleanupConfig struct {
    DB              *sql.DB
    Table           string
    Retention       time.Duration // default: 7 days
    Interval        time.Duration // default: 1 minute
    Column          string        // timestamp column, default: "created_at"
    Condition       string        // optional extra WHERE clause, e.g. "AND published = true"
}

func StartCleanup(ctx context.Context, cfg CleanupConfig) {
    go func() {
        ticker := time.NewTicker(cfg.Interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                cfg.DB.ExecContext(ctx, fmt.Sprintf(
                    "DELETE FROM %s WHERE %s < $1 %s",
                    cfg.Table, cfg.Column, cfg.Condition,
                ), time.Now().Add(-cfg.Retention))
            }
        }
    }()
}
```

For inbox: delete entries older than retention (messages won't be redelivered after broker's max redelivery timeout).

For outbox: delete published entries older than retention (`Condition: "AND published = true"`). Unpublished entries are never cleaned up — they need to be published first.

**Files to Create:**
- `message/sql/cleanup.go`
- `message/sql/cleanup_test.go`

**Acceptance Criteria:**
- [ ] Deletes old entries at configurable interval
- [ ] Configurable retention period (default: 7 days)
- [ ] Outbox cleanup only deletes published entries
- [ ] Stops on context cancellation
- [ ] Does not block or interfere with message processing

## Implementation Order

```
Task 1: Schema Management
  ↓
Task 2: InboxMiddleware    Task 3: OutboxMiddleware
            ↓                         ↓
            └──────────┬──────────────┘
                       ↓
          Task 4: OutboxPublisher
                       ↓
              Task 5: Cleanup
```

Tasks 2 and 3 are independent and can be implemented in parallel. Task 4 depends on Task 3 (needs the outbox table schema). Task 5 is independent but should be done last.

## End-to-End Setup

```go
db, _ := sql.Open("postgres", connStr)

// Initialize schemas (once, at startup)
inbox := msgsql.InboxConfig{DB: db, Table: "processed_messages", Subscriber: "order-service"}
outbox := msgsql.OutboxConfig{DB: db, Table: "outbox", Topic: "order-events"}
msgsql.InitInbox(ctx, inbox)
msgsql.InitOutbox(ctx, outbox)

// Router with full middleware stack
router := message.NewRouter(message.PipeConfig{AckStrategy: message.AckOnSuccess})
router.Use(
    msgsql.TxMiddleware(db, nil),     // outermost user middleware: BEGIN/COMMIT
    msgsql.InboxMiddleware(inbox),     // dedup check
    msgsql.OutboxMiddleware(outbox),   // intercept outputs → outbox table
)
router.AddHandler("place-order", nil, NewPlaceOrderHandler(orderRepo))

// Outbox publisher — separate goroutine
publisher := msgsql.NewOutboxPublisher(msgsql.OutboxPublisherConfig{
    DB:           db,
    Table:        "outbox",
    PollInterval: 500 * time.Millisecond,
    BatchSize:    100,
})
pubCh, _ := publisher.Source(ctx)
// pipe pubCh into broker publisher

// Cleanup — background goroutines
msgsql.StartCleanup(ctx, msgsql.CleanupConfig{
    DB: db, Table: "processed_messages",
    Retention: 7 * 24 * time.Hour, Interval: time.Minute,
})
msgsql.StartCleanup(ctx, msgsql.CleanupConfig{
    DB: db, Table: "outbox",
    Retention: 7 * 24 * time.Hour, Interval: time.Minute,
    Condition: "AND published = true",
})
```

### Execution Flow

```
1. Broker delivers message (msg_id = "abc-123")
2. Acking middleware (outermost) calls next:
   3. TxMiddleware: BEGIN TX
      4. InboxMiddleware: INSERT "abc-123" → new message, proceed
         5. OutboxMiddleware: calls handler
            6. Handler: repo.Save(ctx, order) → adapter uses TxFromContext
            7. Handler returns [OrderPlaced{...}]
         8. OutboxMiddleware: INSERT output into outbox table within TX
         9. OutboxMiddleware: returns nil outputs (swallowed)
      10. InboxMiddleware: returns
   11. TxMiddleware: tx.Commit() → dedup + business state + outbox = atomic
12. Acking middleware: msg.Ack() → broker ack

--- Later, outbox publisher ---
13. OutboxPublisher: SELECT unpublished rows
14. OutboxPublisher: emit RawMessage(OrderPlaced) with acking
15. Broker publisher: publish to "order-events" topic
16. Broker publisher: ack → UPDATE published = true
```

If the same message arrives again:
```
1-3. Same as above
4. InboxMiddleware: INSERT "abc-123" → duplicate key → return nil, nil
5. TxMiddleware: tx.Commit() (no-op, nothing was written)
6. Acking middleware: msg.Ack() → broker ack (message consumed, idempotently)
```

## Open Questions

1. **Row locking during poll**: Should the outbox publisher use `SELECT ... FOR UPDATE SKIP LOCKED` for concurrent publisher instances?
2. **Outbox publisher as pipe**: Should it implement `pipe.Pipe` or just emit a channel? Current design uses `Source()` returning `<-chan *RawMessage`.
3. **pgx error detection**: `isDuplicateKeyError` depends on `pgconn.PgError`. Should we also support `database/sql` driver-specific errors?
4. **Partition-based cleanup**: For high-volume deployments, should we document PostgreSQL table partitioning as an alternative to DELETE-based cleanup?

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] Duplicate messages are idempotently skipped
- [ ] Output events written to outbox within same TX as business state
- [ ] Outbox publisher delivers events to broker
- [ ] Ack on outbox publisher marks row as published
- [ ] Old inbox/outbox entries are cleaned up
- [ ] Schema creation is explicit (not in constructor)
