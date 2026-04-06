# Plan: Message Locals — In-Process Request-Scoped Values

**Status:** Implemented
**Related ADRs:** [0025](../adr/0025-message-context-values.md), [0006](../adr/0006-message-acknowledgment.md), [0022](../adr/0022-message-package-redesign.md)
**Design Evolution:** [locals.decisions.md](locals.decisions.md), [transaction-handling.decisions.md](transaction-handling.decisions.md)
**Unblocks:** [transaction-handling](transaction-handling.md), [inbox-outbox](inbox-outbox.md), auth middleware

## Overview

Add a private `locals map[any]any` field to `TypedMessage` with `SetLocal`/`Local` methods for carrying in-process, request-scoped values. Locals are **decoupled from context**: they do not appear in `ctx.Value()` lookups. Read them via `msg.Local(key)` or typed helpers like `TxFromMessage(msg)`.

Locals are never serialized and naturally die at broker boundaries.

This is general-purpose infrastructure — not tied to any single feature. The same mechanism enables:

| Value | Set by | Used by |
|---|---|---|
| Auth principal | Auth middleware | Authorization checks via `msg.Local(principalKey)` |
| `*sql.Tx` | TxMiddleware | SQL adapters via `TxFromMessage(msg)` |
| Saga state | Saga coordinator | Saga step handlers |

Shipping locals first unblocks auth middleware (next priority) without waiting for the full transaction stack.

## Goals

1. Clean API for carrying in-process values on messages (`SetLocal`/`Local`)
2. Zero-cost when unused (nil map = no allocation, no lookup)
3. Values never serialized, never cross broker boundaries
4. Decoupled from `context.Context` — no merge confusion, no staleness bugs
5. Remove `context.Context` interface resemblance from `TypedMessage`
6. Remove `AttributesFromContext` convenience (use `MessageFromContext(ctx).Attributes`)

## Tasks

### Task 1: Rename Done() -> Settled()

**Status:** Complete

**Goal:** Remove accidental `context.Context` interface resemblance from `TypedMessage`.

**Problem:** `TypedMessage` exposes `Done() <-chan struct{}` and `Err() error` — identical signatures to `context.Context.Done()` and `context.Context.Err()`. But the semantics differ: the message's `Done()` closes on settlement (ack or nack), not on cancellation. The message's `Err()` returns nil after ack, violating the context contract that `Err()` must be non-nil when `Done()` is closed.

**Implementation:**
```go
// Before
func (m *TypedMessage[T]) Done() <-chan struct{}

// After
func (m *TypedMessage[T]) Settled() <-chan struct{}
```

**Files Modified:**
- `message/message.go` — renamed method, updated doc comments
- `message/acking_test.go` — updated all `msg.Done()` -> `msg.Settled()`

### Task 2: Message Locals Field (Decoupled)

**Status:** Complete

**Goal:** Add a private `locals map[any]any` field to `TypedMessage` with `SetLocal`/`Local` methods for in-process values, decoupled from context.

See [withvalue.decisions.md](withvalue.decisions.md) for the full design evolution from the original merge-based `WithValue` approach to the final decoupled `SetLocal`/`Local` design.

**Implementation:**
```go
// message/message.go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    locals     map[any]any  // in-process only, never serialized
}

func (m *TypedMessage[T]) SetLocal(key, val any) {
    if m == nil {
        return
    }
    if m.locals == nil {
        m.locals = make(map[any]any)
    }
    m.locals[key] = val
}

func (m *TypedMessage[T]) Local(key any) any {
    if m == nil {
        return nil
    }
    return m.locals[key]  // nil map read is safe
}

// Copy clones locals (independent copies)
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
    return &TypedMessage[Out]{
        Data:       data,
        Attributes: maps.Clone(msg.Attributes),
        acking:     msg.acking,
        locals:     maps.Clone(msg.locals),
    }
}
```

```go
// message/context.go — simplified messageContext (no merge)
type messageContext struct {
    context.Context
    msg    any
    attrs  Attributes
    expiry time.Time
    // No ctx field — locals are decoupled
}

func (c *messageContext) Value(key any) any {
    switch key {
    case messageKey:
        if msg, ok := c.msg.(*Message); ok { return msg }
        return nil
    case rawMessageKey:
        if msg, ok := c.msg.(*RawMessage); ok { return msg }
        return nil
    default:
        return c.Context.Value(key)
    }
}
```

**Files Modified:**
- `message/message.go` — added `locals` field, `SetLocal`/`Local` methods, updated `Copy`, `Context()`
- `message/context.go` — removed `ctx` merge field, removed `AttributesFromContext`, removed `attributesKey`
- `message/handler.go` — removed `contextWithAttributes` call
- `message/message_test.go` — `TestSetLocal` with 14 subtests
- `message/handler_test.go` — updated to use `MessageFromContext(ctx).Attributes`

### Task 3: AckForward Locals Propagation

**Status:** Complete

**Goal:** When `AckForward` replaces output ackings with SharedAcking, also propagate the message locals from input to output messages.

**Implementation (in forwardAckMiddleware):**
```go
for _, out := range outputs {
    out.acking = shared
    if msg.locals != nil && out.locals == nil {
        out.locals = maps.Clone(msg.locals)
    }
}
```

Propagated locals are **cloned** (independent copies) to prevent cross-message mutation. The guard `out.locals == nil` ensures we don't overwrite locals that a handler explicitly set on its output.

**Files Modified:**
- `message/acking.go` — `forwardAckMiddleware`
- `message/acking_test.go` — `TestForwardAckLocalsPropagation` with 6 subtests

## Breaking Changes (vs v0.17.0)

| Change | Migration |
|---|---|
| `Done()` -> `Settled()` | Rename call sites |
| `SetValue()`/`Value()` -> `SetLocal()`/`Local()` | Rename call sites |
| `AttributesFromContext(ctx)` removed | Use `MessageFromContext(ctx).Attributes` |
| Locals no longer appear in `ctx.Value()` | Use `msg.Local(key)` or `MessageFromContext(ctx).Local(key)` |

## The Two-Layer Model

```
TypedMessage[T]
+-  Data         T              -- payload (serialized)
+-  Attributes   map[string]any -- CloudEvents metadata (serialized)
+-  acking       *Acking        -- acknowledgment lifecycle (in-process)
+-  locals       map[any]any    -- pipeline values (in-process)

          Crosses broker boundary?
Data       yes  (serialized as JSON)
Attributes yes  (serialized as CE attributes)
acking     no   (replaced at broker boundary)
locals     no   (dropped at broker boundary)
```

`Attributes` is the message's serializable identity. `locals` is the message's in-process processing state. They are complementary layers, same as every Go library in the ecosystem (see [design decisions](transaction-handling.decisions.md)).

## Usage Example — Auth Middleware

```go
// Auth middleware — sets principal on message
func AuthMiddleware(verifier TokenVerifier) message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            token := msg.Attributes["authorization"]
            principal, err := verifier.Verify(ctx, token)
            if err != nil {
                return nil, fmt.Errorf("auth: %w", err)
            }
            msg.SetLocal(principalKey, principal)
            return next(ctx, msg)
        }
    }
}

// Typed helper — reads principal from message
func PrincipalFromMessage(msg *message.Message) Principal {
    if p, ok := msg.Local(principalKey).(Principal); ok {
        return p
    }
    return Anonymous
}

// Handler — uses typed helper
func (h *OrderHandler) Handle(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    principal := auth.PrincipalFromMessage(msg)
    if !principal.Can("orders:write") {
        return nil, ErrForbidden
    }
    // ...
}
```

## Acceptance Criteria

- [x] All tasks completed
- [x] Tests pass
- [x] Build passes
- [x] `TypedMessage` does not accidentally satisfy `context.Context`
- [x] Locals not serialized (MarshalJSON / cloudEvent)
- [x] Locals decoupled from handler context
- [x] Nil message safety on SetLocal/Local
- [x] Copy clones locals (independent copies)
- [x] AckForward propagates cloned locals
