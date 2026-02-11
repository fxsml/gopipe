# Plan: Transaction Handling

**Status:** Proposed
**Related ADRs:** [0006](../adr/0006-message-acknowledgment.md), [0022](../adr/0022-message-package-redesign.md)
**Design Evolution:** [transaction-handling.decisions.md](transaction-handling.decisions.md)

## Overview

Add handler-scoped transaction support via middleware. TxMiddleware wraps a handler in BEGIN/COMMIT/ROLLBACK. Adapters extract the TX from context. Handlers remain completely TX-unaware (hexagonal architecture).

This plan also lays the foundation (message context field, middleware ordering) for the [inbox-outbox plan](inbox-outbox.md).

## Goals

1. Handler-scoped TX via middleware (commit before ack, always short-lived)
2. Handlers fully unaware of transactions (hex architecture: adapters use TX, handlers use ports)
3. TX naturally dies at broker boundaries (serialization drops in-process values)
4. No changes to existing handler or acking interfaces
5. Clean API for carrying in-process values on messages (`WithValue`)

## Architecture

```
Broker ──▶ Unmarshal ──▶ Router ──▶ Output/Publisher
                          │
              ┌───────────┴────────────┐
              │  Acking middleware      │  ← outermost (acks after commit)
              │  ┌──────────────────┐  │
              │  │  TxMiddleware    │  │  ← BEGIN TX / COMMIT / ROLLBACK
              │  │  ┌────────────┐  │  │
              │  │  │  Handler   │  │  │  ← TX-unaware, calls ports
              │  │  └────────────┘  │  │
              │  └──────────────────┘  │
              └────────────────────────┘
```

### Component Responsibilities

| Component | Layer | Knows TX? | Responsibility |
|---|---|---|---|
| **TxMiddleware** | Infrastructure | Yes — creates | BEGIN TX, put in context, COMMIT/ROLLBACK |
| **Handler** | Application | **No** | Call ports (interfaces), pass ctx through |
| **Adapter** | Infrastructure | Yes — uses | Extract TX from context, execute queries |
| **Acking** | Framework | No | Ack/nack based on middleware chain result |

### Why Middleware, Not a Pipe

The previous design used a TxStarter pipe that bound TX to acking (ack = commit). This meant TX stayed open for the entire ack lifecycle — potentially seconds to minutes with AckForward. Long-lived transactions cause connection pool exhaustion, lock contention, MVCC bloat, and idle-in-transaction timeouts.

Middleware scopes TX to handler execution — milliseconds for DB operations. Acking remains a separate concern for delivery guarantees.

## Tasks

### Task 0: Rename Done() → Settled()

**Goal:** Remove accidental `context.Context` interface resemblance from `TypedMessage`.

**Problem:** `TypedMessage` exposes `Done() <-chan struct{}` and `Err() error` — identical signatures to `context.Context.Done()` and `context.Context.Err()`. But the semantics differ: the message's `Done()` closes on settlement (ack or nack), not on cancellation. The message's `Err()` returns nil after ack, violating the context contract that `Err()` must be non-nil when `Done()` is closed.

If a type looks like a context, it should fulfill the contract. Since the message is NOT a context (it carries one, creates one, but is not one), the method should be renamed to avoid confusion.

**Implementation:**
```go
// Before
func (m *TypedMessage[T]) Done() <-chan struct{}

// After
func (m *TypedMessage[T]) Settled() <-chan struct{}
```

`Err()` stays — it's a common Go method name (`sql.Rows.Err()`, `bufio.Scanner.Err()`). It's `Done() <-chan struct{}` that creates the distinctive `context.Context` resemblance.

**Files to Modify:**
- `message/message.go` — rename method, update doc comments
- `message/acking_test.go` — update all `msg.Done()` → `msg.Settled()`

**Impact:** No production code outside the message package calls `msg.Done()`. All call sites are in tests and doc comments within the package.

**Acceptance Criteria:**
- [ ] `Done()` renamed to `Settled()` on `TypedMessage`
- [ ] All tests updated and passing
- [ ] Doc comments updated
- [ ] `TypedMessage` does not accidentally satisfy `context.Context`

### Task 1: Middleware Ordering — Acking Outermost

**Goal:** Move acking middleware from innermost to outermost so that user middleware (TX, inbox, outbox) runs INSIDE acking scope.

**Problem:** Currently the router applies middleware as: `User → Acking → Handler`. User middleware wraps acking. This means acking fires BEFORE user middleware finishes:

```
TxMiddleware (user, outer):
  BEGIN TX
  acking (inner):
    handler()
    msg.Ack()    ← broker ack fires HERE
  tx.Commit()    ← commit happens AFTER ack — if commit fails, data lost
```

**Solution:** Reverse the order to: `Acking → User → Handler`. Acking wraps user middleware:

```
acking (outer):
  TxMiddleware (user, inner):
    BEGIN TX
    handler()
    tx.Commit()  ← commit happens HERE
  msg.Ack()      ← broker ack fires AFTER commit — safe
```

This matches Watermill's model where the subscriber/router controls acking around the entire processing chain.

**Implementation:**
```go
// router.go — Pipe() method
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    // ...

    // Apply user middleware: first registered wraps outermost
    fn := r.process
    for i := len(r.middleware) - 1; i >= 0; i-- {
        fn = r.middleware[i](fn)
    }

    // Apply acking strategy as outermost middleware (wraps everything)
    fn = r.ackingMiddleware()(fn)

    // ...
}
```

**Files to Modify:**
- `message/router.go` — swap middleware application order in `Pipe()`
- `message/router_test.go` — verify middleware ordering behavior

**Acceptance Criteria:**
- [ ] Acking middleware applied outermost (after user middleware)
- [ ] User middleware runs inside acking scope
- [ ] AckOnSuccess: handler error → nack, handler success → ack (unchanged semantics)
- [ ] AckForward: SharedAcking set on final outputs from middleware chain
- [ ] AckManual: unchanged behavior
- [ ] Existing tests pass

### Task 2: Message Context Field

**Goal:** Add a `context.Context` field to the message for in-process, request-scoped values. Values auto-propagate into the handler's context via `msg.Context(parent)`.

See [transaction-handling.decisions.md](transaction-handling.decisions.md) for full design evaluation.

**Summary:** Add an unexported `ctx context.Context` field to `TypedMessage` with a `WithValue` method for adding values. The `messageContext.Value()` method checks this field before delegating to parent, making values automatically available to handlers and adapters. No bridging middleware needed.

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

// WithValue adds an in-process value to the message's context.
// Values auto-propagate into contexts created by Context(parent).
// They are NOT serialized and do NOT cross broker boundaries.
// Multiple calls chain: each value is added to the existing context.
func (m *TypedMessage[T]) WithValue(key, val any) {
    if m.ctx == nil {
        m.ctx = context.WithValue(context.Background(), key, val)
    } else {
        m.ctx = context.WithValue(m.ctx, key, val)
    }
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
// message/context.go — modified messageContext
type messageContext struct {
    context.Context
    msg    any
    attrs  Attributes
    expiry time.Time
    ctx    context.Context  // message's in-process values
}

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
- `message/message.go` — add `ctx` field, `WithValue` method, update `Copy`, update `Context()`
- `message/context.go` — add `ctx` to `messageContext`, modify `Value()`, update constructor
- `message/message_test.go` — test value propagation through Context()

**Acceptance Criteria:**
- [ ] `msg.WithValue(key, val)` stores in-process values on message
- [ ] Multiple `WithValue` calls chain (additive, not replacement)
- [ ] `msg.Context(parent).Value(key)` returns stored values
- [ ] Values not included in MarshalJSON / cloudEvent()
- [ ] Copy() propagates ctx to output messages
- [ ] Zero-cost when unused (nil ctx = no allocation, no lookup)

### Task 3: SQL Transaction Package

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

### Task 4: TxMiddleware

**Goal:** Handler-scoped transaction middleware. BEGIN before handler, COMMIT on success, ROLLBACK on error.

**Implementation:**
```go
// message/sql/tx_middleware.go
func TxMiddleware(db *sql.DB, opts *sql.TxOptions) message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            tx, err := db.BeginTx(ctx, opts)
            if err != nil {
                return nil, fmt.Errorf("begin tx: %w", err)
            }

            msg.WithValue(ctxKey{}, tx)

            outputs, err := next(ctx, msg)
            if err != nil {
                tx.Rollback()
                return nil, err
            }

            if err := tx.Commit(); err != nil {
                return nil, fmt.Errorf("commit tx: %w", err)
            }

            return outputs, nil
        }
    }
}
```

**Key design:** TxMiddleware wraps the handler in BEGIN/COMMIT/ROLLBACK. It uses `msg.WithValue` to attach the TX to the message. The handler's adapter extracts it via `TxFromContext(ctx)` — the TX auto-propagates into handler context through `messageContext.Value()`.

With Task 1's middleware reordering, acking fires AFTER this middleware returns, ensuring commit-before-ack.

**Files to Create:**
- `message/sql/tx_middleware.go`
- `message/sql/tx_middleware_test.go`

**Acceptance Criteria:**
- [ ] TX begins before handler, commits after handler success
- [ ] TX rolls back on handler error
- [ ] TX available to adapters via `TxFromContext(ctx)` inside handler
- [ ] Commit happens before broker ack (depends on Task 1)
- [ ] Handler code has zero SQL/TX imports

### Task 5: AckForward Context Propagation

**Goal:** When `AckForward` replaces output ackings with SharedAcking, also propagate the message context from input to output messages.

**Why:** The `commandHandler` creates output messages via `New(event, attrs, nil)` (handler.go:135). These outputs have no context values. With AckForward, trace context and other pipeline values should flow to downstream handlers.

Note: With handler-scoped TX, the TX is already committed by the time outputs reach downstream handlers. Context propagation here is for non-TX values (traces, auth, saga state).

**Implementation (in forwardAckMiddleware):**
```go
for _, out := range outputs {
    out.acking = shared
    if msg.ctx != nil && out.ctx == nil {
        out.ctx = msg.ctx
    }
}
```

The guard `out.ctx == nil` ensures we don't overwrite values that a handler explicitly set on its output.

**Files to Modify:**
- `message/acking.go` — `forwardAckMiddleware`

**Acceptance Criteria:**
- [ ] Output messages inherit input's context values when AckForward is used
- [ ] AckOnSuccess and AckManual are unaffected
- [ ] Explicitly set output context is not overwritten

## Implementation Order

```
Task 0: Rename Done() → Settled()
  ↓
Task 1: Middleware Ordering     Task 2: Message Context Field     Task 3: SQL Package
            ↓                               ↓                         ↓
            └───────────────────────────────┴─────────────────────────┘
                                            ↓
                                   Task 4: TxMiddleware
                                            ↓
                              Task 5: AckForward Context Propagation
```

Task 0 is a prerequisite (clean API before adding to it). Tasks 1, 2, and 3 are independent and can be implemented in parallel. Task 4 depends on all three. Task 5 depends on Task 2.

## End-to-End Trace

### Setup
```go
db, _ := sql.Open("postgres", connStr)

router := message.NewRouter(message.PipeConfig{
    AckStrategy: message.AckOnSuccess,
})
router.Use(msgsql.TxMiddleware(db, nil))
router.AddHandler("place-order", nil, NewPlaceOrderHandler(orderRepo))
```

### Execution Flow
```
1. Broker delivers message
2. Router receives message
3. Acking middleware (outermost) calls next:
   4. TxMiddleware: BEGIN TX, msg.WithValue(txKey, tx)
      5. Handler: repo.Save(ctx, order) — adapter calls TxFromContext(ctx)
      6. Handler returns outputs
   7. TxMiddleware: tx.Commit()
8. Acking middleware: msg.Ack() → broker ack (AFTER commit)
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

1. **pgx support**: Should the Executor interface and context helpers also support `pgx.Tx`? Or is `database/sql` sufficient for now?
2. **Savepoints**: Should nested TX middleware create savepoints instead of failing on `BeginTx`?

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] Handler code has zero SQL/TX imports
- [ ] TX commits before broker ack
- [ ] TX reference not serialized (MarshalJSON)
- [ ] `TypedMessage` does not accidentally satisfy `context.Context`
