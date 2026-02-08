# Plan: Transaction Handling

**Status:** Proposed
**Related ADRs:** [0006](../adr/0006-message-acknowledgment.md), [0022](../adr/0022-message-package-redesign.md)
**Design Evolution:** [transaction-handling.decisions.md](transaction-handling.decisions.md)

## Overview

Add transaction support to the message package using a pipeline-based approach: a TxStarter pipe begins the transaction and binds commit/rollback to the message's acking, middleware injects the TX into handler context for adapters, and AckForward cascades the TX lifecycle across processing steps.

Handlers remain completely TX-unaware (hexagonal architecture). Adapters extract the TX from context.

## Goals

1. Transaction lifecycle managed via acking (ack = commit, nack = rollback)
2. Handlers fully unaware of transactions (hex architecture: adapters use TX, handlers use ports)
3. TX scope controlled by existing ack strategy (AckOnSuccess = single handler, AckForward = pipeline-wide)
4. TX naturally dies at broker boundaries (serialization drops in-process values)
5. No changes to existing handler, middleware, or acking interfaces

## Pipeline Architecture

```
Broker ──▶ Unmarshal ──▶ TxStarter ──▶ Router ──▶ Output/Publisher
  │                        │             │              │
  │                     begin TX     middleware       ack output
  │                     wrap acking  injects TX       cascades back
  │                                  into context     to TxStarter's
  │                                  for adapters     acking → commit
  │
  acking = broker ack/nack
```

### Component Responsibilities

| Component | Layer | Knows TX? | Responsibility |
|---|---|---|---|
| **TxStarter pipe** | Infrastructure | Yes — creates | Begin TX, wrap acking, attach TX to message |
| **InjectTx middleware** | Infrastructure | Yes — bridges | Read TX from message, put in context |
| **Handler** | Application | **No** | Call ports (interfaces), pass ctx through |
| **Adapter** | Infrastructure | Yes — uses | Extract TX from context, execute queries |
| **AckForward** | Framework | No | Cascade ack to downstream outputs |
| **Output publisher** | Infrastructure | No | Ack after successful publish → cascades commit |

### Ack Strategy = TX Scope

The existing ack strategy controls TX scope with no additional configuration:

| AckStrategy | TX Scope | Use Case |
|---|---|---|
| `AckOnSuccess` | Single handler | Simple CRUD: process event, write to DB, commit |
| `AckForward` | Entire downstream pipeline | Saga: commit only after all downstream steps succeed |
| `AckManual` | Handler-controlled | Complex flows requiring explicit commit control |

## Tasks

### Task 1: Message Context Field

**Goal:** Add a `context.Context` field to the message for in-process, request-scoped values. Values auto-propagate into the handler's context via `msg.Context(parent)`.

See [transaction-handling.decisions.md](transaction-handling.decisions.md) for full design evaluation.

**Summary:** Following the Watermill pattern (`SetContext` / `Context`), add an unexported `ctx context.Context` field to `TypedMessage`. The `messageContext.Value()` method checks this field before delegating to parent, making values automatically available to handlers and adapters. No bridging middleware needed.

This is general purpose — not just for TX. The same field carries trace spans, auth principals, saga state, or any in-process value that should flow with the message but die at broker boundaries.

**Implementation:**
```go
// message/message.go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    ctx        context.Context  // in-process only, never serialized
}

func (m *TypedMessage[T]) SetContext(ctx context.Context) {
    m.ctx = ctx
}

// Copy propagates ctx
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
    return &TypedMessage[Out]{
        Data:       data,
        Attributes: maps.Clone(msg.Attributes),
        acking:     msg.acking,
        ctx:        msg.ctx,
    }
}
```

```go
// message/context.go — modified messageContext.Value()
func (c *messageContext) Value(key any) any {
    switch key {
    case messageKey:
        if msg, ok := c.msg.(*Message); ok { return msg }
        return nil
    case rawMessageKey:
        if msg, ok := c.msg.(*RawMessage); ok { return msg }
        return nil
    case attributesKey:
        return c.attrs
    default:
        if c.ctx != nil {
            if v := c.ctx.Value(key); v != nil {
                return v
            }
        }
        return c.Context.Value(key)
    }
}
```

**Files to Modify:**
- `message/message.go` — add `ctx` field, `SetContext` method, update `Copy`
- `message/context.go` — add `ctx` to `messageContext`, modify `Value()`, update `Context()`
- `message/message_test.go` — test value propagation through Context()

**Acceptance Criteria:**
- [ ] `msg.SetContext(ctx)` stores in-process context on message
- [ ] `msg.Context(parent).Value(key)` returns stored values
- [ ] Values not included in MarshalJSON / cloudEvent()
- [ ] Copy() propagates ctx to output messages
- [ ] Zero-cost when unused (nil ctx = no allocation, no lookup)

### Task 2: SQL Transaction Package

**Goal:** Provide TX context helpers and the Executor interface for adapters.

**Implementation:**
```go
// message/sql/context.go
package sql

type ctxKey struct{}

func ContextWithTx(ctx context.Context, tx *sql.Tx) context.Context {
    return context.WithValue(ctx, ctxKey{}, tx)
}

func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
    tx, ok := ctx.Value(ctxKey{}).(*sql.Tx)
    return tx, ok
}
```

```go
// message/sql/executor.go
type Executor interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ExecutorFromContext(ctx context.Context, fallback *sql.DB) Executor {
    if tx, ok := TxFromContext(ctx); ok {
        return tx
    }
    return fallback
}
```

**Files to Create:**
- `message/sql/context.go` — TX context helpers
- `message/sql/executor.go` — Executor interface
- `message/sql/context_test.go`
- `message/sql/executor_test.go`

**Acceptance Criteria:**
- [ ] `ContextWithTx` / `TxFromContext` round-trip works
- [ ] `ExecutorFromContext` returns TX when present, fallback when absent
- [ ] Both `*sql.DB` and `*sql.Tx` satisfy `Executor`

### Task 3: TxStarter Pipe

**Goal:** Pipeline component that begins a transaction and wraps the message's acking with commit/rollback.

**Implementation:**
```go
// message/sql/tx_starter.go
type TxStarter struct {
    db   *sql.DB
    opts *sql.TxOptions
}

func NewTxStarter(db *sql.DB, opts *sql.TxOptions) *TxStarter {
    return &TxStarter{db: db, opts: opts}
}

func (s *TxStarter) Process(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    tx, err := s.db.BeginTx(ctx, s.opts)
    if err != nil {
        return nil, fmt.Errorf("begin tx: %w", err)
    }

    // New acking: commit/rollback wraps original broker ack/nack
    txAcking := message.NewAcking(
        func()        { tx.Commit(); msg.Ack() },
        func(e error) { tx.Rollback(); msg.Nack(e) },
    )

    // New message with TX acking. Original msg keeps broker acking.
    out := message.New(msg.Data, maps.Clone(msg.Attributes), txAcking)
    out.SetContext(context.WithValue(context.Background(), ctxKey{}, tx))

    return []*message.Message{out}, nil
}
```

**Key design:** The TxStarter creates a NEW acking that composes TX lifecycle with the original. It calls the public `msg.Ack()` / `msg.Nack()` on the original message from inside the new acking's callbacks — same pattern as `forwardAckMiddleware`.

**Files to Create:**
- `message/sql/tx_starter.go`
- `message/sql/tx_starter_test.go`

**Acceptance Criteria:**
- [ ] TxStarter begins TX on process
- [ ] Output message's ack triggers commit then broker ack
- [ ] Output message's nack triggers rollback then broker nack
- [ ] TX reference available on output message via WithValue
- [ ] Original input message's acking unchanged

### Task 4: ~~InjectTx Middleware~~ (eliminated)

With `SetContext` and auto-propagation via `messageContext.Value()`, the TX is already in the handler's context. No bridging middleware needed. The TxStarter sets `context.WithValue(ctx, txKey, tx)` on the message, and `msg.Context(parent)` includes it automatically.

This task is eliminated by the design choice in Task 1.

### Task 5: Pipeline Value Propagation in AckForward

**Goal:** When `AckForward` replaces output ackings with SharedAcking, also propagate pipeline values from input to output messages.

**Why:** The `commandHandler` creates output messages via `New(event, attrs, nil)` (handler.go:135). These outputs have no pipeline values. With AckForward, the TX lifecycle already spans the outputs (via SharedAcking). The TX reference should also be available to downstream handlers.

**Implementation (in forwardAckMiddleware):**
```go
for _, out := range outputs {
    out.acking = shared
    if msg.ctx != nil {
        out.ctx = msg.ctx  // propagate pipeline values
    }
}
```

**Files to Modify:**
- `message/acking.go` — `forwardAckMiddleware`

**Acceptance Criteria:**
- [ ] Output messages inherit input's pipeline values when AckForward is used
- [ ] AckOnSuccess and AckManual are unaffected
- [ ] Downstream handlers can access TX from context

## Implementation Order

```
Task 1: Message Context Field    Task 2: SQL Package
            ↓                         ↓
            └──────────┬──────────────┘
                       ↓
              Task 3: TxStarter Pipe
                       ↓
         Task 5: AckForward Context Propagation
```

Tasks 1 and 2 are independent and can be implemented in parallel. Task 4 (middleware) is eliminated by auto-propagation.

## End-to-End Trace

### Setup
```go
db, _ := sql.Open("postgres", connStr)
txStarter := msgsql.NewTxStarter(db, nil)

router := message.NewRouter(message.PipeConfig{
    AckStrategy: message.AckForward,
})
router.AddHandler("place-order", nil, NewPlaceOrderHandler(orderRepo))
```

### Pipeline
```go
// Broker delivers message
brokerAcking := message.NewAcking(
    func()       { consumer.Ack(offset) },
    func(e error) { consumer.Nack(offset) },
)
raw := message.NewRaw(payload, attrs, brokerAcking)

// Unmarshal → TxStarter → Router → Output
//
// TxStarter wraps acking:
//   ack  → tx.Commit()  → consumer.Ack(offset)
//   nack → tx.Rollback() → consumer.Nack(offset)
//
// Router processes with TX in context (auto-propagated via SetContext)
// Handler calls repo.Save(ctx, order) — adapter uses TxFromContext
// Output publisher acks → cascades → tx.Commit() → consumer.Ack()
```

### Handler (application layer — TX-unaware)
```go
func NewPlaceOrderHandler(repo OrderRepository) message.Handler {
    return message.NewCommandHandler(
        func(ctx context.Context, cmd PlaceOrder) ([]OrderPlaced, error) {
            order := NewOrder(cmd)
            if err := repo.Save(ctx, order); err != nil {
                return nil, err
            }
            return []OrderPlaced{{OrderID: order.ID}}, nil
        },
        message.CommandHandlerConfig{Source: "/orders"},
    )
}
```

### Adapter (infrastructure layer — TX-aware)
```go
type PostgresOrderRepo struct{ db *sql.DB }

func (r *PostgresOrderRepo) Save(ctx context.Context, order Order) error {
    exec := msgsql.ExecutorFromContext(ctx, r.db)
    _, err := exec.ExecContext(ctx,
        "INSERT INTO orders (id, product_id, qty) VALUES ($1, $2, $3)",
        order.ID, order.ProductID, order.Quantity,
    )
    return err
}
```

## Open Questions

1. **Outbox pattern**: Should the outbox middleware intercept output messages and write them to an outbox table within the same TX? This is a natural extension but adds complexity (forwarder daemon). Defer to a separate plan.
2. **TX timeout**: Long-running pipelines with AckForward keep the TX open. Should TxStarter accept a TX-level timeout via `sql.TxOptions` or context deadline?
3. **Savepoints**: Should nested TxStarter pipes create savepoints instead of new transactions?
4. **pgx support**: Should the Executor interface and context helpers also support `pgx.Tx`? Or is `database/sql` sufficient for now?

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] Handler code has zero SQL/TX imports
- [ ] TX commits on ack, rolls back on nack
- [ ] TX reference not serialized (MarshalJSON)
- [ ] AckForward cascades TX lifecycle to downstream outputs
