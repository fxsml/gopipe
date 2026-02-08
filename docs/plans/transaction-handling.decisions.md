# Transaction Handling - Design Evolution: Message Pipeline Context

**Status:** Open
**Related Plan:** [transaction-handling.md](transaction-handling.md)

## Context

The TxStarter pipe creates a TX and binds commit/rollback to the message's acking. Downstream components (adapters) need to access this TX reference. The TX must travel with the message through the in-process pipeline but must NOT cross broker boundaries (serialization).

The core question: **how should a message carry in-process values that downstream components can read?**

A secondary but equally important question: **what is this field for, and what should it be called?**

---

## Naming Analysis: What Is This Field?

### What It Is NOT

**It is not "local."** No Go library uses this term. "Local to what?" is unanswerable — the message? The handler? The process? The term has no precedent and no clear scope.

**It is not "extensions."** CloudEvents uses "extensions" for additional *serializable* attributes (like `correlationid`, `expirytime`). gopipe already handles these in `Attributes`. In-process values are a different concern entirely.

**It is not "metadata."** gRPC and Watermill use "metadata" for *serializable* key-value pairs that travel on the wire (gRPC's `metadata.MD`, Watermill's `Metadata map[string]string`). gopipe's `Attributes` already serves this role.

### What It IS

It is the message's `context.Context` — the same thing that `http.Request`, Watermill, and every Go request-carrying struct uses for in-process, request-scoped values.

### Ecosystem Precedent

Every major Go library separates serializable metadata from in-process context using the same two-layer pattern:

| Library | Serializable (wire) | In-process (Go memory) |
|---|---|---|
| `net/http` | `Header` (`http.Header`) | `ctx` (`context.Context`) |
| gRPC-Go | `metadata.MD` | `context.Context` values |
| Watermill | `Metadata` (`map[string]string`) | `ctx` (`context.Context`) |
| Sarama (Kafka) | `Headers` (`[]*RecordHeader`) | Session provides `Context()` |
| NATS | `Header` (`http.Header`) | Connection methods accept `ctx` |
| CloudEvents | `Attributes` + `Extensions` | Not specified (SDK-level) |

**gopipe already has the first layer**: `Attributes` (serializable CloudEvents context attributes). The missing second layer is `context.Context` (in-process values).

### The Scope Is General Purpose

This is not just for transactions. The same mechanism carries:

| Value | Set by | Used by |
|---|---|---|
| `*sql.Tx` | TxStarter pipe | SQL adapters via `TxFromContext(ctx)` |
| Trace span | OTel middleware/pipe | Instrumented adapters via `trace.SpanFromContext(ctx)` |
| Logger | Logging pipe | Adapters via `slog.FromContext(ctx)` (hypothetical) |
| Auth principal | Auth middleware/pipe | Authorization adapters |
| Saga state | Saga coordinator pipe | Saga step adapters |

All of these are in-process, request-scoped values that should flow with the message but die at broker boundaries. `context.Context` is Go's standard mechanism for exactly this.

### The "Context on a Struct" Concern

The `context` package documentation says: *"Do not store Contexts inside a struct type."*

The Go community recognizes explicit exceptions:

1. **`http.Request.Context()`** — the standard library itself stores a context on a struct when the struct IS the request. ([Go blog: Contexts and structs](https://go.dev/blog/context-and-structs))

2. **Message-on-a-channel** — Jack Lindamood's influential post identifies this as THE exception: *"The one exception to not storing a context is when you need to put it in a struct that is used purely as a message that is passed across a channel."* ([How to correctly use context.Context in Go 1.7](https://medium.com/@cep21/how-to-correctly-use-context-context-in-go-1-7-8f2c0fafdf39))

3. **Brad Fitzpatrick (Go team)** in [issue #22602](https://github.com/golang/go/issues/22602): *"While we've told people not to add contexts to structs, I think that guidance is over-aggressive. The real advice is not to store contexts. They should be passed along like parameters. But if the struct is essentially just a parameter, it's okay."*

The principle is: **context must flow, not be stored.** If the struct itself flows (message through a pipeline), context-on-struct is appropriate. If the struct persists (service, repository), context should not be on it. `TypedMessage` flows — it is gopipe's request type, equivalent to `http.Request`.

**Watermill validates this directly.** Its `message.Message` has both `Metadata map[string]string` and `ctx context.Context`, with `Context()` / `SetContext()` methods. This is the exact same pattern proposed here.

---

## Options Considered

### Option A: Explicit Key-Value Map (`SetLocal` / `Local`)

Add a `map[any]any` field with getter/setter. Bridging middleware reads from map, injects into context.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    local      map[any]any
}

func (m *TypedMessage[T]) SetLocal(key, val any) { ... }
func (m *TypedMessage[T]) Local(key any) (any, bool) { ... }
```

**Pros:**
- O(1) lookup and deletion
- Can enumerate values

**Cons:**
- Invents a parallel value mechanism alongside `context.Context` — no precedent in Go ecosystem
- Requires bridging middleware per router (ceremony to shuttle data between containers)
- Silent failure mode: forget the middleware → adapter silently falls back to `*sql.DB`
- "Local" naming has no precedent — vague, unclear scope
- Two APIs for the same value: `msg.Local(txKey)` vs `TxFromContext(ctx)`

### Option B: context.Context Field with Custom `WithValue`

Add a `context.Context` field. Provide a `WithValue` convenience method. Auto-propagate into `msg.Context(parent)`.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    ctx        context.Context
}

func (m *TypedMessage[T]) WithValue(key, val any) {
    if m.ctx == nil {
        m.ctx = context.WithValue(context.Background(), key, val)
    } else {
        m.ctx = context.WithValue(m.ctx, key, val)
    }
}
```

**Pros:**
- Auto-propagation via `messageContext.Value()` — no bridging middleware
- Uses Go's standard value mechanism

**Cons:**
- `WithValue` is a non-standard method name — `context.WithValue` is a package-level function, not a method. Naming a method `WithValue` creates false familiarity
- Hides the `context.Context` behind a custom API — callers can't use standard context composition (`context.WithValue`, `context.WithCancel`, etc.)
- Cannot read the stored context to build on it from a separate pipeline component

### Option C: context.Context Field with `SetContext` (Watermill Pattern)

Add a `context.Context` field with `SetContext()` setter. The existing `Context(parent)` auto-propagates values. Matches Watermill's proven API exactly.

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    acking     *Acking
    ctx        context.Context  // in-process only, never serialized
}

func (m *TypedMessage[T]) SetContext(ctx context.Context) {
    m.ctx = ctx
}
```

Usage:
```go
// TxStarter
out.SetContext(context.WithValue(context.Background(), txKey, tx))

// Trace middleware (later in pipeline, builds on existing)
ctx := out.Context(context.Background()) // derives context including stored values
out.SetContext(context.WithValue(ctx, spanKey, span))

// Adapter (unchanged)
tx, _ := TxFromContext(ctx)
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
        if c.ctx != nil {
            if v := c.ctx.Value(key); v != nil {
                return v
            }
        }
        return c.Context.Value(key)
    }
}
```

**Pros:**
- Proven pattern: Watermill uses identical API (`SetContext` / `Context`)
- Idiomatic: callers use standard `context.WithValue` — no custom API to learn
- Auto-propagation: values automatically available in handler context via `msg.Context(parent)`
- No bridging middleware needed
- Single lookup path for adapters: `ctx.Value(key)` — standard Go
- Zero-cost when unused: `ctx == nil` → no allocation, no lookup
- Naturally dies at broker boundaries: `ctx` not included in `MarshalJSON` / `cloudEvent()`
- Composable: multiple pipeline components can build on the stored context
- General purpose: TX, traces, saga state, auth — same mechanism for all

**Cons:**
- Cannot enumerate or delete stored values (context doesn't support it)
- ALL handlers see stored values — no opt-in per router. But adapters that don't need them simply don't look them up.

### Option D: Attributes Extension (Reuse Existing Map)

Store in-process values in `Attributes` with naming convention.

**Rejected:** Attributes serialize to JSON. `*sql.Tx` cannot serialize. Breaks CloudEvents spec.

### Option E: Interface-Based Extensions Field

Add a single `any` field for user-defined structs.

**Rejected:** Not composable across pipeline components.

---

## Analysis

### Why Option C over Option B

Option B invents a `WithValue` method that looks like `context.WithValue` but behaves differently (mutates in place, manages the `context.Background()` root). This is a trap: it creates false familiarity with a non-standard API.

Option C uses `SetContext` — a clear setter — and leaves context construction to standard library functions. The caller writes `context.WithValue(...)` themselves. No custom API to learn. This matches Watermill exactly.

### The Two-Layer Model for gopipe

```
TypedMessage[T]
├── Data         T              — payload (serialized)
├── Attributes   map[string]any — CloudEvents metadata (serialized)
├── acking       *Acking        — acknowledgment lifecycle (in-process)
└── ctx          context.Context — pipeline values (in-process)

          Crosses broker boundary?
Data       ✓  (serialized as JSON)
Attributes ✓  (serialized as CE attributes)
acking     ✗  (replaced at broker boundary)
ctx        ✗  (dropped at broker boundary)
```

`Attributes` is the message's serializable identity. `ctx` is the message's in-process processing state. They are complementary layers, same as every Go library in the ecosystem.

### Copy() Behavior

`Copy()` should propagate `ctx` because:
- With `AckForward`, the TX lifecycle already spans output messages (via SharedAcking)
- Downstream handlers processing those outputs should see the same pipeline values
- If they don't need them, values being in context is harmless

`New()` should NOT set `ctx` because:
- New messages start fresh
- Pipeline values are set explicitly by components like TxStarter

### AckForward Value Propagation

`forwardAckMiddleware` creates output acking but does not create output messages — handlers do. The handler creates outputs via `New(event, attrs, nil)` (handler.go:135), which starts with nil `ctx`.

For pipeline values to reach downstream handlers via AckForward, the middleware should propagate `ctx` from input to output:

```go
for _, out := range outputs {
    out.acking = shared
    if msg.ctx != nil && out.ctx == nil {
        out.ctx = msg.ctx
    }
}
```

The guard `out.ctx == nil` ensures we don't overwrite values that a handler explicitly set on its output.

### OpenTelemetry Integration

OpenTelemetry propagates trace context across process boundaries via `TextMapCarrier`:

```
Producer: context.Context --[Inject]--> message headers --[wire]--> ...
Consumer: ... --[wire]--> message headers --[Extract]--> context.Context
```

For gopipe, this maps naturally:
- **Cross-broker:** `Inject` writes trace headers into `Attributes` before serialization. `Extract` reads them into `ctx` after deserialization. The `TextMapCarrier` interface wraps `Attributes`.
- **In-process:** The span lives in `ctx`, automatically available to handlers and adapters via `msg.Context(parent)`.

No special mechanism needed — the two-layer model handles it.

---

## Recommendation

**Option C: `context.Context` field with `SetContext`, auto-propagated via `messageContext.Value()`.**

Rationale:
1. Proven pattern — Watermill uses identical API on its `Message` type
2. Idiomatic Go — standard `context.WithValue` for construction, standard `ctx.Value()` for retrieval
3. Consistent with existing `messageContext` — extends `Value()` with one more layer
4. General purpose — TX, traces, saga state, auth all use the same mechanism
5. No bridging middleware — eliminates ceremony and the silent failure mode
6. The naming matches Go convention: `context.Context` on a request struct, exposed via `SetContext`

API surface:
```go
// One new method on TypedMessage
func (m *TypedMessage[T]) SetContext(ctx context.Context)

// One modified internal method
func (c *messageContext) Value(key any) any  // adds ctx lookup

// Copy() propagates ctx (one line)
// forwardAckMiddleware propagates ctx (one line)
```

## Open Concerns

1. **Value shadowing**: If both `ctx` and the parent context contain the same key, `ctx` wins. This is intentional (pipeline values override infrastructure defaults) but should be documented.
2. **Thread safety**: `SetContext` replaces `m.ctx` (pointer swap). Safe under single-writer assumption (one worker processes one message). Same assumption as Attributes mutation.
3. **Value cleanup**: Cannot remove values from a context chain. For TX and traces, this is fine — they are valid for the message's entire in-process lifetime. If future use cases need removal, revisit.

## Sources

- [Go blog: Contexts and structs](https://go.dev/blog/context-and-structs)
- [Go issue #22602: context in structs](https://github.com/golang/go/issues/22602)
- [Jack Lindamood: context in Go 1.7](https://medium.com/@cep21/how-to-correctly-use-context-context-in-go-1-7-8f2c0fafdf39)
- [Watermill message.go](https://github.com/ThreeDotsLabs/watermill/blob/master/message/message.go)
- [OpenTelemetry propagation package](https://pkg.go.dev/go.opentelemetry.io/otel/propagation)
- [CloudEvents spec](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md)
