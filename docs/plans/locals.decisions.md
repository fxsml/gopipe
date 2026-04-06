# Message Locals — Design Evolution: From Merge to Decouple

**Status:** Decided
**Related Plan:** [locals.md](locals.md)
**Prior Design:** [transaction-handling.decisions.md](transaction-handling.decisions.md)

## Context

The original plan ([transaction-handling.decisions.md](transaction-handling.decisions.md)) proposed adding a `context.Context` field to `TypedMessage` with a `WithValue` method. Values would auto-propagate into handler contexts via `messageContext.Value()` — the **merge design**. During implementation, several problems emerged that led to a fundamentally different approach.

This document captures the design evolution: what we tried, what broke, and why we ended up with decoupled locals.

---

## Phase 1: WithValue (Merge Design)

The initial implementation followed the plan exactly:

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    ctx        context.Context  // in-process values
}

func (m *TypedMessage[T]) WithValue(key, val any) {
    if m.ctx == nil {
        m.ctx = context.WithValue(context.Background(), key, val)
    } else {
        m.ctx = context.WithValue(m.ctx, key, val)
    }
}
```

The `messageContext.Value()` method consulted `c.ctx` before the parent context:

```go
func (c *messageContext) Value(key any) any {
    // ... special keys (messageKey, rawMessageKey, attributesKey) ...
    default:
        if c.ctx != nil {
            if v := c.ctx.Value(key); v != nil {
                return v
            }
        }
        return c.Context.Value(key)
}
```

### Rename: WithValue -> SetValue + Value getter

`WithValue` implies immutability (returns a new thing, like `context.WithValue`). But the method mutates `m.ctx` in place. Renamed to `SetValue` for honesty about mutation. Added `Value()` getter for symmetry — typed helpers like `TxFromContext(ctx)` need a way to read message-bound values when they have a `*Message` but not the merged context.

---

## Phase 2: Problems Discovered

### Problem 1: Staleness Bug

`messageContext` captures `msg.ctx` at construction time:

```go
func (m *TypedMessage[T]) Context(parent context.Context) context.Context {
    return &messageContext{
        Context: parent,
        ctx:     m.ctx,  // snapshot
    }
}
```

If `SetValue` is called after `Context()`, the `messageContext` holds a stale pointer. The new value is invisible through `ctx.Value()`. Confirmed with a scratch test:

```go
msg := New("data", nil, nil)
ctx := msg.Context(context.Background())
msg.SetValue(key, "value")
ctx.Value(key)  // returns nil — stale snapshot
```

This is a fundamental timing bug in the merge design. The only fix would be to store a `*Message` reference in `messageContext` and do live lookup — which creates a cyclic dependency between the context and the message.

### Problem 2: Shadowing Semantics Confusion

With merge, message values shadow parent context values:

```go
parent := context.WithValue(bg, key, "infrastructure")
msg.SetValue(key, "pipeline")
ctx := msg.Context(parent)
ctx.Value(key)  // returns "pipeline" — shadows parent
```

This is intentional per the original design ("pipeline values override infrastructure defaults"). But it creates a non-obvious contract: whether `ctx.Value(key)` returns a value depends on whether some middleware called `SetValue` with the same key. The shadowing is invisible at the call site.

### Problem 3: False Symmetry with ctx.Value

Naming the getter `Value()` creates an expectation that `msg.Value(key)` and `ctx.Value(key)` return the same thing. They don't:

- `msg.Value(key)` — message-local only
- `ctx.Value(key)` — merged view (message values + parent context values)

Two APIs, same vocabulary, different semantics. A persistent source of confusion.

### Problem 4: Third-Party Interop Illusion

The merge design was partly motivated by the idea that `msg.SetValue(otelKey, span)` would make `trace.SpanFromContext(ctx)` work automatically. It doesn't — OTel (and most libraries) use unexported key types. You can't call `msg.SetValue(otel's-unexported-key, span)` from outside the package.

Bridging middleware is needed regardless:
```go
// Must use OTel's own API to inject into context
ctx = trace.ContextWithSpan(ctx, span)
```

This means the "auto-propagation" benefit of merge is largely theoretical for the most important use case. The real consumers — typed helpers we control — work equally well with direct message access.

---

## Phase 3: Decouple Decision

### The Insight

Middleware operates on `*Message` directly. Handlers receive `(ctx, msg)`. Adapters that need pipeline values (TX, auth) can access them via `MessageFromContext(ctx).Local(key)` or, more commonly, via typed helpers that accept `*Message`.

The merge design's auto-propagation solves a problem that mostly doesn't exist: the common consumer patterns already have access to the message.

### Decouple vs Merge — Final Evaluation

| Concern | Merge (ctx.Value) | Decouple (msg.Local) |
|---|---|---|
| Staleness bug | Yes — snapshot capture | None — direct map read |
| Shadowing | Implicit, non-obvious | None — separate namespaces |
| API confusion | `msg.Value` vs `ctx.Value` | `msg.Local` — clearly different |
| Third-party interop | Doesn't work (unexported keys) | Doesn't work (same reason) |
| Typed helpers | Work via either path | Work via message access |
| Complexity | messageContext merge logic | Simple map |
| Enumeration | Not possible (context chain) | Possible (map iteration) |
| Deletion | Not possible | Possible (`delete(m.locals, k)`) |
| O(1) lookup | No (context chain walk) | Yes (map) |

Decouple wins on every axis except "values automatically appear in ctx.Value()" — a benefit that is largely theoretical given unexported key types in third-party libraries.

### Naming: Value -> Local

With decoupling, the name must clearly signal "this is not context":

- `Value` — rejected: mirrors `ctx.Value()`, false symmetry
- `Local` — chosen: "local to this message, local to this process"

The original [transaction-handling.decisions.md](transaction-handling.decisions.md) rejected "local" as having "no precedent." On re-evaluation: the lack of precedent is actually a feature. `Local` does not overlap with any existing Go concept (`context.Value`, `http.Header`, `metadata.MD`). It signals something new: message-scoped, in-process, non-serializable state. The name earns its meaning from the API.

### Internal Representation: map vs context.Context chain

With decouple, the internal field no longer needs to be a `context.Context`. Options:

| Concern | `context.Context` chain | `map[any]any` |
|---|---|---|
| Lookup | O(N) walk | O(1) |
| Allocs per Set | 1 (new valueCtx node) | Amortized ~0 after init |
| Enumerable | No | Yes |
| Delete a key | No | Yes |
| Zero-cost when unused | nil, no alloc | nil, no alloc |
| Copy semantics | Shared pointer (diverges safely) | Must `maps.Clone` |

Map wins. The context-chain representation was inherited from the merge design; with decouple it's pure overhead.

### Public Field vs Private Methods

Considered making `Locals` a public `map[any]any` field (consistent with public `Attributes`). Rejected because:

1. **Key type safety**: `any` keys (context convention for collision avoidance via unexported types) don't play well with a public map — invites string-key collisions.
2. **Reassignment risk**: A public field lets callers nil it, reassign it, or clear it — lifecycle violations. Worse with `context.Context` field type: `msg.Locals = someCancelableCtx` injects lifecycle into a value-only field.
3. **Representation freedom**: Private field lets us switch from map to slice or other structure if profiling demands.
4. **Justified asymmetry with Attributes**: `Attributes` is public because it's CloudEvents-defined, serialized (walked during marshaling), has a constrained key type (`string`), and external code legitimately iterates it. `Locals` is the opposite: never serialized, never enumerated by callers, opaque `any` keys, internal-only.

Two methods (`SetLocal`/`Local`) with nil-message guards, matching the `Acking` safety pattern.

### Removing AttributesFromContext

`AttributesFromContext(ctx)` was a convenience that stored attributes separately in context via `attributesKey`. With `MessageFromContext(ctx)` already returning the full message, the convenience is redundant:

```go
// Before
attrs := message.AttributesFromContext(ctx)

// After
attrs := message.MessageFromContext(ctx).Attributes
```

One fewer context key, one fewer concept. The message reference in context is sufficient.

### Removing contextWithAttributes from handler.go

The `commandHandler.Handle` method called `contextWithAttributes(ctx, msg.Attributes)` to inject attributes into the handler's context. With `AttributesFromContext` removed, this call serves no purpose. The handler already receives context built from `msg.Context(parent)` upstream, which stores the message reference.

---

## Decision

**Decouple locals from context. Use `SetLocal`/`Local` backed by `map[any]any`.**

The merge design's auto-propagation benefit was largely theoretical (unexported third-party keys prevent it from working where it matters most). The costs were real: staleness bug, shadowing confusion, false API symmetry, O(N) lookup, no enumeration/deletion.

Decoupled locals are simpler, faster, and honest about what they are: message-scoped state that lives alongside — but separate from — Go's context mechanism.

### API Surface

```go
// Two new methods on TypedMessage
func (m *TypedMessage[T]) SetLocal(key, val any)
func (m *TypedMessage[T]) Local(key any) any

// Simplified messageContext.Value() — no merge logic
func (c *messageContext) Value(key any) any  // messageKey, rawMessageKey, parent only

// Copy() clones locals
// forwardAckMiddleware clones locals to outputs

// Removed:
// - AttributesFromContext
// - contextWithAttributes
// - attributesKey constant
```

### The Two-Layer Model (Updated)

```
TypedMessage[T]
+-  Data         T              -- payload (serialized)
+-  Attributes   map[string]any -- CloudEvents metadata (serialized)
+-  acking       *Acking        -- acknowledgment lifecycle (in-process)
+-  locals       map[any]any    -- pipeline values (in-process, decoupled from ctx)

          Crosses broker boundary?
Data       yes  (serialized as JSON)
Attributes yes  (serialized as CE attributes)
acking     no   (replaced at broker boundary)
locals     no   (dropped at broker boundary)
```

### Typed Helper Pattern

```go
// Package auth
type principalKeyType struct{}
var principalKey principalKeyType

func SetPrincipal(msg *message.Message, p Principal) {
    msg.SetLocal(principalKey, p)
}

func PrincipalFromMessage(msg *message.Message) Principal {
    if p, ok := msg.Local(principalKey).(Principal); ok {
        return p
    }
    return Anonymous
}

// In handler context (msg not directly available)
func PrincipalFromContext(ctx context.Context) Principal {
    msg := message.MessageFromContext(ctx)
    if msg == nil {
        return Anonymous
    }
    return PrincipalFromMessage(msg)
}
```

## Open Concerns

1. **messageContext Deadline override**: `messageContext.Deadline()` reports message expiry without enforcing it — a contract violation (`Deadline()` implies `Done()` will fire). Consider dropping the override entirely and relying solely on `middleware.Deadline()` for enforcement. This would allow deleting `messageContext` and using plain `context.WithValue` for message/rawMessage keys.
2. **Thread safety**: `SetLocal` mutates `m.locals` (map write). Safe under single-writer assumption (one worker processes one message at a time). Same assumption as Attributes mutation.
3. **Value cleanup**: Map supports `delete()` but no public API for it yet. Not needed for current use cases (TX, auth, saga state are valid for the message's entire in-process lifetime). Add if needed.

## Sources

- [Go blog: Contexts and structs](https://go.dev/blog/context-and-structs)
- [Watermill message.go](https://github.com/ThreeDotsLabs/watermill/blob/master/message/message.go) — studied their `SetContext`/`Context` pattern and intentionally diverged
- [transaction-handling.decisions.md](transaction-handling.decisions.md) — original design evaluation that proposed the merge approach
