# ADR 0025: Message Locals for In-Process Values

**Date:** 2026-04-04
**Status:** Implemented
**Supersedes:** Original proposal (WithValue with context merge)

## Context

TxMiddleware, tracing middleware, auth middleware, and saga coordinators all need to carry request-scoped values that travel with a message through the pipeline but must NOT cross broker boundaries (no serialization). Examples: `*sql.Tx`, trace spans, auth principals, saga state.

The `Attributes` field handles serializable CloudEvents metadata. There is no established layer for in-process values. Every Go library in the ecosystem solves this with a two-layer model — serializable metadata + in-process state — but `TypedMessage` only has the first layer.

Separately, `TypedMessage` currently exposes `Done() <-chan struct{}` and `Err() error` — the same signatures as `context.Context`. But the semantics differ: `Done()` closes on settlement (ack/nack) not cancellation, and `Err()` returns nil after ack (violating the context contract that `Err()` is non-nil when `Done()` is closed). The type accidentally satisfies `context.Context` without honoring its contract.

## Decision

Add a private `locals map[any]any` field to `TypedMessage` with `SetLocal`/`Local` methods. Locals are **decoupled from context**: they do not appear in `ctx.Value()` lookups. Rename `Done()` → `Settled()` to remove the accidental context resemblance. Remove `AttributesFromContext` (use `MessageFromContext(ctx).Attributes`).

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    locals     map[any]any  // in-process only, never serialized
}

// SetLocal adds an in-process value. Backed by map for O(1) lookup.
func (m *TypedMessage[T]) SetLocal(key, val any)

// Local reads an in-process value. Returns nil if not set.
func (m *TypedMessage[T]) Local(key any) any

// Rename: Done() → Settled()
func (m *TypedMessage[T]) Settled() <-chan struct{}
```

**Two-layer model:**

| Field | Crosses broker boundary? | Purpose |
|---|---|---|
| `Data` | yes (serialized) | payload |
| `Attributes` | yes (CloudEvents attrs) | serializable metadata |
| `acking` | no (replaced at boundary) | acknowledgment lifecycle |
| `locals` | no (dropped at boundary) | in-process values |

**Decoupled design:** The original proposal merged locals into handler contexts via `messageContext.Value()`. This was rejected due to: staleness bugs (snapshot capture), shadowing confusion, false API symmetry with `ctx.Value()`, and the discovery that third-party interop (OTel, etc.) doesn't work through merge anyway (unexported key types). See [locals.decisions.md](../plans/locals.decisions.md) for the full design evolution.

**Map over context chain:** With decoupling, the internal representation uses `map[any]any` instead of `context.Context` chain — O(1) lookup, enumerable, supports deletion, fewer allocations.

**No implicit propagation:** Locals are never implicitly copied from input to output messages by the framework. Acking (delivery guarantees) and locals (in-process state) are orthogonal concerns — coupling them would make handler behavior silently change based on the router's ack strategy. Handlers that need to carry locals forward use `Copy()` (clones all locals) or `SetLocal()` (specific keys) explicitly.

**Context-on-struct exception:** The Go community recognizes message-on-a-channel as the standard exception to "don't store context in structs" (Brad Fitzpatrick in golang/go#22602; Jack Lindamood; `http.Request` itself). While we no longer store a `context.Context`, the principle applies: `TypedMessage` is gopipe's request type — carrying in-process state on it is appropriate.

## Consequences

**Breaking Changes:**
- `TypedMessage.Done()` renamed to `Settled()` — callers must update
- `SetValue()`/`Value()` renamed to `SetLocal()`/`Local()`
- `AttributesFromContext(ctx)` removed — use `MessageFromContext(ctx).Attributes`
- Locals do not appear in `ctx.Value()` — use `msg.Local(key)` or `MessageFromContext(ctx).Local(key)`

**Benefits:**
- Single mechanism for all in-process pipeline values (TX, traces, auth, saga)
- O(1) lookup via map (vs O(N) context chain walk)
- Zero-cost when unused (nil map = no allocation, no lookup)
- Values naturally die at broker boundaries (not in `MarshalJSON`/`cloudEvent`)
- No staleness bugs (direct map read, not snapshot)
- No shadowing confusion (locals and context are separate namespaces)
- `TypedMessage` no longer accidentally satisfies `context.Context`
- Nil message safety on SetLocal/Local (matching Acking pattern)

**Drawbacks:**
- Locals not automatically visible in `ctx.Value()` — requires explicit access via message
- Copy must clone the map (same pattern as Attributes)

## Links

- Plan: [locals](../plans/locals.md)
- Design evolution: [locals.decisions.md](../plans/locals.decisions.md)
- Prior design analysis: [transaction-handling.decisions.md](../plans/transaction-handling.decisions.md)
- Related: ADR 0006 (Message Acknowledgment), ADR 0022 (Message Package Redesign)
- Unblocks: [transaction-handling plan](../plans/transaction-handling.md), [inbox-outbox plan](../plans/inbox-outbox.md)
