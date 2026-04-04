# Plan: WithValue — Message Context for In-Process Values

**Status:** Proposed
**Related ADRs:** [0006](../adr/0006-message-acknowledgment.md), [0022](../adr/0022-message-package-redesign.md)
**Design Evolution:** [transaction-handling.decisions.md](transaction-handling.decisions.md)
**Unblocks:** [transaction-handling](transaction-handling.md), [inbox-outbox](inbox-outbox.md), auth middleware

## Overview

Add a `context.Context` field to `TypedMessage` with a `WithValue` method for carrying in-process, request-scoped values. Values auto-propagate into the handler's context via `msg.Context(parent)`. They are never serialized and naturally die at broker boundaries.

This is general-purpose infrastructure — not tied to any single feature. The same mechanism enables:

| Value | Set by | Used by |
|---|---|---|
| Auth principal | Auth middleware | Authorization adapters |
| `*sql.Tx` | TxMiddleware | SQL adapters via `TxFromContext(ctx)` |
| Trace span | OTel middleware | Instrumented adapters via `trace.SpanFromContext(ctx)` |
| Saga state | Saga coordinator | Saga step adapters |

Shipping WithValue first unblocks auth middleware (next priority) without waiting for the full transaction stack.

## Goals

1. Clean API for carrying in-process values on messages (`WithValue`)
2. Auto-propagation into handler context — no bridging middleware needed
3. Zero-cost when unused (nil ctx = no allocation, no lookup)
4. Values never serialized, never cross broker boundaries
5. Remove `context.Context` interface resemblance from `TypedMessage`

## Tasks

### Task 1: Rename Done() → Settled()

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

### Task 2: Message Context Field

**Goal:** Add a `context.Context` field to the message for in-process, request-scoped values. Values auto-propagate into the handler's context via `msg.Context(parent)`.

See [transaction-handling.decisions.md](transaction-handling.decisions.md) for full design evaluation.

**Summary:** Add an unexported `ctx context.Context` field to `TypedMessage` with a `WithValue` method for adding values. The `messageContext.Value()` method checks this field before delegating to parent, making values automatically available to handlers and adapters. No bridging middleware needed.

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

### Task 3: AckForward Context Propagation

**Goal:** When `AckForward` replaces output ackings with SharedAcking, also propagate the message context from input to output messages.

**Why:** The `commandHandler` creates output messages via `New(event, attrs, nil)` (handler.go:135). These outputs have no context values. With AckForward, pipeline values (traces, auth, saga state) should flow to downstream handlers.

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
Task 1: Rename Done() → Settled()
  ↓
Task 2: Message Context Field
  ↓
Task 3: AckForward Context Propagation
```

Task 1 is a prerequisite (clean API surface before adding context field). Task 3 depends on Task 2 (needs the `ctx` field to exist).

## The Two-Layer Model

```
TypedMessage[T]
├── Data         T              — payload (serialized)
├── Attributes   map[string]any — CloudEvents metadata (serialized)
├── acking       *Acking        — acknowledgment lifecycle (in-process)
└── ctx          context.Context — pipeline values (in-process)

          Crosses broker boundary?
Data       yes  (serialized as JSON)
Attributes yes  (serialized as CE attributes)
acking     no   (replaced at broker boundary)
ctx        no   (dropped at broker boundary)
```

`Attributes` is the message's serializable identity. `ctx` is the message's in-process processing state. They are complementary layers, same as every Go library in the ecosystem (see [design decisions](transaction-handling.decisions.md)).

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
            msg.WithValue(principalKey, principal)
            return next(ctx, msg)
        }
    }
}

// Adapter — extracts principal from context
func (r *Repo) Save(ctx context.Context, order Order) error {
    principal := auth.PrincipalFromContext(ctx) // uses ctx.Value()
    if !principal.Can("orders:write") {
        return ErrForbidden
    }
    // ...
}
```

## Open Concerns

1. **Value shadowing**: If both `ctx` and the parent context contain the same key, `ctx` wins. This is intentional (pipeline values override infrastructure defaults) but should be documented.
2. **Thread safety**: `WithValue` mutates `m.ctx` (pointer swap via `context.WithValue`). Safe under single-writer assumption (one worker processes one message). Same assumption as Attributes mutation.
3. **Value cleanup**: Cannot remove values from a context chain. For TX, traces, and auth, this is fine — they are valid for the message's entire in-process lifetime.

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)
- [ ] `TypedMessage` does not accidentally satisfy `context.Context`
- [ ] ctx not serialized (MarshalJSON / cloudEvent)
- [ ] Values auto-propagate into handler context
