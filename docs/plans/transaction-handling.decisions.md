# Transaction Handling - Design Evolution: Message Pipeline Values

**Status:** Open
**Related Plan:** [transaction-handling.md](transaction-handling.md)

## Context

The TxStarter pipe creates a TX and binds commit/rollback to the message's acking. Downstream components (middleware, adapters) need to access this TX reference. The TX must travel with the message through the in-process pipeline but must NOT cross broker boundaries (serialization).

The core question: **how should a message carry in-process values that downstream components can read?**

---

## Options Considered

### Option A: Explicit Key-Value Map (`SetLocal` / `Local`)

Add a `map[any]any` field to the message with getter/setter methods. A bridging middleware reads from the map and injects into context.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    local      map[any]any  // in-process only, never serialized
}

func (m *TypedMessage[T]) SetLocal(key, val any) {
    if m.local == nil { m.local = make(map[any]any) }
    m.local[key] = val
}

func (m *TypedMessage[T]) Local(key any) (any, bool) {
    if m.local == nil { return nil, false }
    v, ok := m.local[key]
    return v, ok
}
```

Usage:
```go
// TxStarter
out.SetLocal(txKey, tx)

// Bridging middleware (required per router)
func InjectTx() message.Middleware {
    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            if tx, ok := msg.Local(txKey); ok {
                ctx = ContextWithTx(ctx, tx.(*sql.Tx))
            }
            return next(ctx, msg)
        }
    }
}

// Adapter
tx, _ := TxFromContext(ctx)
```

**Pros:**
- Simple, explicit API
- O(1) lookup and deletion
- Clear separation between message values and context values
- Minimal surface area

**Cons:**
- Creates a parallel value-carrying mechanism alongside `context.Context`
- Requires bridging middleware per router — ceremony that exists only to move data from one container to another
- Not idiomatic Go: context is THE standard mechanism for request-scoped values
- `Local` naming is vague — local to what?
- Two lookups: adapter calls `TxFromContext(ctx)`, middleware calls `msg.Local(txKey)` — different APIs for the same value

### Option B: context.Context Field with Auto-Propagation

Add a `context.Context` field to the message for pipeline-scoped values. The `messageContext.Value()` method checks this field before delegating to parent, making values automatically available in any context created by `msg.Context(parent)`.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    pipeCtx    context.Context  // in-process pipeline values, never serialized
}

func (m *TypedMessage[T]) WithValue(key, val any) {
    if m.pipeCtx == nil {
        m.pipeCtx = context.WithValue(context.Background(), key, val)
    } else {
        m.pipeCtx = context.WithValue(m.pipeCtx, key, val)
    }
}
```

Modified `messageContext.Value()`:
```go
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
        if c.pipeCtx != nil {
            if v := c.pipeCtx.Value(key); v != nil {
                return v
            }
        }
        return c.Context.Value(key)
    }
}
```

Usage:
```go
// TxStarter
out.WithValue(txKey, tx)

// NO middleware needed

// Adapter (unchanged)
tx, _ := TxFromContext(ctx)
```

**Pros:**
- Idiomatic Go: uses `context.Context` for request-scoped values, THE standard mechanism
- No bridging middleware: values auto-propagate into handler context via `msg.Context(parent)`
- Consistent with existing `messageContext` design: it already overrides `Value()` for attributes and message references — pipeline values are the same pattern, one more layer in the chain
- Single lookup path: adapter calls `ctx.Value(key)`, same as any Go code
- Zero-cost when unused: `pipeCtx == nil` → no allocation, no lookup
- Naturally dies at broker boundaries: `pipeCtx` is not included in `MarshalJSON` / `cloudEvent()`

**Cons:**
- Cannot enumerate or delete values (context doesn't support it)
- Allocates a context chain (`context.Background()` → `WithValue` → ...) — minor, one allocation per `WithValue` call
- ALL handlers see the TX in context, no opt-in per router — but adapters that don't need TX simply don't call `TxFromContext`, so this is harmless
- Storing `context.Context` on a struct is unusual in Go (contexts are normally passed as function arguments, not stored) — see analysis below

### Option C: Attributes Extension (Reuse Existing Map)

Store in-process values in the existing `Attributes` map with a naming convention (e.g., `_tx`).

```go
msg.Attributes["_tx"] = tx
```

**Pros:**
- No new API at all
- Already exists

**Cons:**
- Attributes serialize to JSON: `*sql.Tx` cannot serialize, will panic or corrupt
- Pollutes CloudEvents attributes with non-CE data
- No type safety
- Breaks the CloudEvents spec contract
- **Rejected: fundamentally incompatible with serialization**

### Option D: Interface-Based Extensions Field

Add a single `any` field for user-defined extension structs.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    Extensions any  // user-defined struct
}
```

**Pros:**
- Type-safe if users define their own struct
- Simple field access

**Cons:**
- Not composable: only one struct per message, so different pipeline components cannot independently add values
- Requires type assertion at every access point
- No standard lookup pattern
- **Rejected: not composable**

---

## Analysis

### The "context.Context on a struct" concern

Go convention says contexts should be passed as function arguments, not stored on structs. The `context` package documentation states: *"Do not store Contexts inside a struct type."*

However, this guidance exists for a specific reason: stored contexts can outlive their intended scope, leading to goroutine leaks or stale cancellation. Our case is different:

1. **The pipeline context carries only values, not cancellation.** It is rooted at `context.Background()` — it never cancels, never has a deadline. It is purely a value container.
2. **The message IS the request.** In HTTP servers, context is tied to the request lifecycle. In gopipe, the message is the request. Storing pipeline values on the message is semantically equivalent to `http.Request.Context()` — which stores a context on a struct.
3. **`http.Request.Context()` is the precedent.** The standard library itself stores a context on a struct when the struct IS the request. `TypedMessage` is gopipe's request type.

The guideline is about not storing *cancellation-bearing* contexts on *long-lived* structs. A value-only context on a per-request message does not violate the spirit of the rule.

### Why auto-propagation matters

Consider what happens without auto-propagation (Option A):

```go
// TxStarter sets value
out.SetLocal(txKey, tx)

// Router 1: needs InjectTx middleware
router1.Use(InjectTx())

// Router 2: also needs InjectTx middleware
router2.Use(InjectTx())

// Router 3: forgot InjectTx middleware — TX silently absent, adapter falls back to *sql.DB
// Bug: writes happen outside the transaction, no error reported
router3.Use(/* oops */)
```

With auto-propagation (Option B), the TX is in context regardless. There is nothing to forget. The failure mode of "adapter silently uses wrong connection" cannot happen.

### Copy() behavior

`Copy()` should propagate `pipeCtx` because:
- With `AckForward`, the TX lifecycle already spans output messages (via SharedAcking)
- Downstream handlers processing those outputs should see the same TX
- If they don't need it, the TX being in context is harmless

`New()` should NOT set `pipeCtx` because:
- New messages start fresh
- Pipeline values are set explicitly by components like TxStarter

### AckForward value propagation

`forwardAckMiddleware` creates output messages' acking but does not create the output messages themselves — handlers do. The handler creates outputs via `New(event, attrs, nil)` (handler.go:135), which starts with nil `pipeCtx`.

For the TX to reach downstream handlers via AckForward, the middleware should propagate `pipeCtx` from input to output:

```go
for _, out := range outputs {
    out.acking = shared
    if msg.pipeCtx != nil && out.pipeCtx == nil {
        out.pipeCtx = msg.pipeCtx
    }
}
```

The guard `out.pipeCtx == nil` ensures we don't overwrite values that a handler explicitly set on its output.

---

## Recommendation

**Option B: context.Context field with auto-propagation.**

Rationale:
1. It follows how `messageContext` already works — extending `Value()` with one more layer
2. It eliminates bridging middleware that exists only to shuttle data between containers
3. It uses Go's standard value propagation mechanism
4. The `http.Request.Context()` precedent validates storing a context on a request struct
5. The failure mode of "forgot to add middleware, TX silently absent" disappears

The API surface is minimal:
```go
// One new method on TypedMessage
func (m *TypedMessage[T]) WithValue(key, val any)

// One modified method (internal)
func (c *messageContext) Value(key any) any  // adds pipeCtx lookup

// Copy() propagates pipeCtx (one line change)
// forwardAckMiddleware propagates pipeCtx (one line change)
```

### What we gain vs Option A

| Concern | Option A (SetLocal) | Option B (WithValue) |
|---|---|---|
| API additions on message | `SetLocal`, `Local` | `WithValue` |
| Middleware needed | `InjectTx` per router | None |
| Adapter code | Same (`TxFromContext`) | Same (`TxFromContext`) |
| Failure mode | Silent fallback if middleware forgotten | Cannot happen |
| Idiomatic Go | Parallel mechanism | Uses context.Context |
| Value availability | Explicit bridge | Automatic |

## Open Concerns

1. **Value shadowing**: If both `pipeCtx` and the parent context contain the same key, `pipeCtx` wins. This is intentional (pipeline values override infrastructure defaults) but should be documented.
2. **Thread safety**: `WithValue` replaces `m.pipeCtx` (pointer swap). Safe under single-writer assumption (one worker processes one message). Same assumption as Attributes mutation.
3. **Value cleanup**: Cannot remove values from a context chain. For TX, this is fine — the TX is valid for the message's entire lifetime. If future use cases need removal, revisit.
