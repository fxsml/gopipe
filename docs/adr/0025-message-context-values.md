# ADR 0025: Message Context for In-Process Values

**Date:** 2026-04-04
**Status:** Proposed

## Context

TxMiddleware, tracing middleware, auth middleware, and saga coordinators all need to carry request-scoped values that travel with a message through the pipeline but must NOT cross broker boundaries (no serialization). Examples: `*sql.Tx`, trace spans, auth principals, saga state.

The `Attributes` field handles serializable CloudEvents metadata. There is no established layer for in-process values. Every Go library in the ecosystem solves this with a two-layer model — serializable metadata + `context.Context` — but `TypedMessage` only has the first layer.

Separately, `TypedMessage` currently exposes `Done() <-chan struct{}` and `Err() error` — the same signatures as `context.Context`. But the semantics differ: `Done()` closes on settlement (ack/nack) not cancellation, and `Err()` returns nil after ack (violating the context contract that `Err()` is non-nil when `Done()` is closed). The type accidentally satisfies `context.Context` without honoring its contract.

## Decision

Add an unexported `ctx context.Context` field to `TypedMessage` with a `WithValue` method. Auto-propagate values into handler context via `messageContext.Value()`. Rename `Done()` → `Settled()` to remove the accidental context resemblance.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    ctx        context.Context  // in-process only, never serialized
}

// WithValue adds an in-process value. Multiple calls chain additively.
// Values auto-propagate into msg.Context(parent) and are dropped at broker boundaries.
func (m *TypedMessage[T]) WithValue(key, val any) {
    if m.ctx == nil {
        m.ctx = context.WithValue(context.Background(), key, val)
    } else {
        m.ctx = context.WithValue(m.ctx, key, val)
    }
}

// Rename: Done() → Settled()
func (m *TypedMessage[T]) Settled() <-chan struct{}
```

**Two-layer model:**

| Field | Crosses broker boundary? | Purpose |
|---|---|---|
| `Data` | yes (serialized) | payload |
| `Attributes` | yes (CloudEvents attrs) | serializable metadata |
| `acking` | no (replaced at boundary) | acknowledgment lifecycle |
| `ctx` | no (dropped at boundary) | in-process values |

**Additive API over replacement:** `WithValue` chains like `context.WithValue` rather than replacing the stored context. Multiple independent callers (TxMiddleware, OTel, auth) can each call `WithValue` without coordinating — the safe path is the only path. Contrast Watermill's `SetContext(ctx)`, where a second caller must read-compose-set, and forgetting to read silently discards prior values.

**Context-on-struct exception:** The Go community recognizes message-on-a-channel as the standard exception to "don't store context in structs" (Brad Fitzpatrick in golang/go#22602; Jack Lindamood; `http.Request` itself). `TypedMessage` is gopipe's request type — equivalent to `http.Request`, which stores a context the same way.

## Consequences

**Breaking Changes:**
- `TypedMessage.Done()` renamed to `Settled()` — callers must update (internal tests only in current tree)

**Benefits:**
- Single mechanism for all in-process pipeline values (TX, traces, auth, saga)
- Auto-propagation into handler context — no bridging middleware
- Zero-cost when unused (nil ctx = no allocation, no lookup)
- Values naturally die at broker boundaries (not in `MarshalJSON`/`cloudEvent`)
- Additive API prevents silent value loss between middleware
- `TypedMessage` no longer accidentally satisfies `context.Context`

**Drawbacks:**
- `WithValue` is a non-standard method name (`context.WithValue` is a package-level function)
- Cannot enumerate or delete stored values (inherited from `context.Context`)
- All handlers see all stored values — no per-router scoping (but adapters that don't look them up are unaffected)

## Links

- Plan: [withvalue](../plans/withvalue.md)
- Design evolution: [transaction-handling.decisions.md](../plans/transaction-handling.decisions.md)
- Related: ADR 0006 (Message Acknowledgment), ADR 0022 (Message Package Redesign)
- Unblocks: [transaction-handling plan](../plans/transaction-handling.md), [inbox-outbox plan](../plans/inbox-outbox.md)
