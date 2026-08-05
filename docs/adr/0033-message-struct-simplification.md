# ADR 0033: Message Struct Simplification

**Date:** 2026-08-05
**Status:** Implemented

## Context

ADR 0010 (Dual Message Types) introduced `TypedMessage[T]` as the base generic type, with `Message = TypedMessage[any]` and `RawMessage = TypedMessage[[]byte]` as type aliases: `Message` for typed pipeline stages, `RawMessage` for broker/pub-sub boundaries. `UnmarshalPipe`/`MarshalPipe` (`message/pipes.go`) convert between them at the raw/typed boundary and are in production use.

Issue #125 identified a real cost of the dual-type system: middleware written against one instantiation (`Middleware[*RawMessage, *Message]` for unmarshal, `Middleware[*Message, *RawMessage]` for marshal, `Middleware[*Message, *Message]` for handlers, `Middleware[*RawMessage, *RawMessage]` for proxying) cannot compose across the others — four incompatible signatures for what is conceptually one message flowing through a pipeline. `docs/plans/marshaling-strategy.md` records the design work that led to this ADR; see that document for the full options analysis (including approaches later superseded by [ADR 0031](0031-handler-level-marshaling.md)).

`Engine` (ADR 0020/0022), the one component that depended most heavily on `TypedMessage[T]`, was removed first — see [ADR 0032](0032-remove-message-engine.md) — specifically to unblock this change without spending effort porting orchestration code already out of scope.

**Why not always-`[]byte`, like Watermill's `Message.Payload`?** Watermill's core message type carries `Payload []byte` permanently — no generic or `any` parameter, ever. Typed access exists only inside `components/cqrs`, where a command/event value is a transient function argument, never written back into persistent message state. gopipe's `Data any` is deliberately not that: it is *persistent, addressable* message state that can carry a typed Go value across multiple pipeline stages — e.g. a handler configured to skip marshaling, feeding further typed processing, before an explicit `MarshalPipe` stage downstream (`marshaling-strategy.md` §3). Collapsing to always-`[]byte` would foreclose that multi-stage typed composition use case entirely, not just change its ergonomics. This is also *why* a runtime `Raw()` check is needed at all — Watermill never needs one, because the raw-vs-typed ambiguity it guards against doesn't exist in a payload-is-always-bytes design.

## Decision

### 1. `Message` becomes a concrete struct; `TypedMessage[T]`/`RawMessage` are dropped

```go
type Message struct {
    Data       any
    Attributes Attributes
    acking     *Acking
    locals     map[any]any
}

// Raw reports whether Data currently holds raw []byte, and returns it.
func (m *Message) Raw() ([]byte, bool) {
    b, ok := m.Data.([]byte)
    return b, ok
}
```

No generic parameter, no `RawMessage` alias, no `NewTyped[T]`/`NewRaw`. `New(data any, attrs, acking) *Message` is the only constructor. `Copy(msg *Message, data any) *Message` loses its generic type parameters.

`Raw()` follows Go's comma-ok idiom (`v, ok := x.(T)`, `context.Deadline() (time.Time, bool)`, `sync.Map.Load`) rather than a bool-only `IsRaw()`: it both answers "is this raw" and, when the answer is yes, hands back the bytes in the same call — `IsRaw() bool` alone would force a second type assertion to actually use the data.

### 2. `UnmarshalPipe`/`MarshalPipe` port to `*Message` → `*Message`, failing loudly on mismatch

Both pipes keep their existing public API shape (constructors, `Pipe`, `Use`, `Stats`) but their channel type changes from `*RawMessage`↔`*Message` to a uniform `*Message`→`*Message`, using `Raw()` internally:

```go
func (p *UnmarshalPipe) process(ctx context.Context, msg *Message) ([]*Message, error) {
    raw, ok := msg.Raw()
    if !ok {
        return nil, fmt.Errorf("%w: got %T", ErrDataNotRaw, msg.Data)
    }
    // ... unmarshal raw into a typed instance, msg.Data = instance
}
```

`MarshalPipe` is symmetric, returning `ErrDataNotTyped` if `Data` is already raw `[]byte` when it reaches the pipe — a message arriving at `MarshalPipe` already-raw means something upstream marshaled it, or the wrong channel feeds this stage, not a legitimate shortcut. Both pipes now accept `message.Middleware` (the same type `Router` uses) instead of the generic `pipe/middleware.Middleware[In, Out]`, since In and Out are now identical.

Losing the compile-time guarantee that `UnmarshalPipe` can only be fed `*RawMessage` is a deliberate trade: the whole point of collapsing the type is to enforce raw-vs-typed state as a runtime invariant instead of a type-system one, and `ErrDataNotRaw`/`ErrDataNotTyped` are what make that enforcement loud instead of silent.

### 3. `message/jsonschema`, `message/http`, `message/cloudevents` retype mechanically

`NewValidationMiddleware`, `NewInputValidationMiddleware`, `NewOutputValidationMiddleware` move from generic `pipe/middleware.Middleware[*RawMessage, ...]` instantiations to the single `message.Middleware` type, reading bytes via `Raw()` instead of direct field access; `NewOutputValidationMiddleware` additionally checks `Raw()` on its output (a pre-existing gap — see marshaling-strategy.md's Final Design §3 for the double-encoding scenario this class of check exists to prevent). `message/http` and `message/cloudevents` (`FromCloudEvent`/`ToCloudEvent`, `Subscriber`, `Publisher`) are pure `*RawMessage`→`*Message` retypes with no behavior change.

`message/context.go`'s `RawMessageFromContext` is dropped — `MessageFromContext` already covers the single-type world.

## Consequences

**Breaking Changes:**
- `TypedMessage[T]`, `RawMessage`, `NewTyped[T]`, `NewRaw` removed. `New(data any, attrs, acking) *Message` is the only constructor.
- `Copy[In, Out any]` becomes `Copy(msg *Message, data any) *Message`.
- `ParseRaw`/`parseRawBytes` return `*Message` (with `Data []byte`) instead of `*RawMessage`.
- `UnmarshalPipe`/`MarshalPipe` change from `*RawMessage`↔`*Message` to uniform `*Message`→`*Message`, and now fail with `ErrDataNotRaw`/`ErrDataNotTyped` on state mismatch instead of relying on the compiler to rule mismatches out. Their `Use()` now takes `message.Middleware` instead of generic `pipe/middleware.Middleware[In, Out]`.
- `message.RawMessageFromContext` removed; use `MessageFromContext`.
- `message/jsonschema`'s three middleware constructors, `message/http` (`Subscriber`/`Publisher`), and `message/cloudevents` (`FromCloudEvent`/`ToCloudEvent`/`Subscriber`/`Publisher`) retype from `*RawMessage` to `*Message`.
- External consumers typed on `RawMessage` (`gopipe-azservicebus`'s entire public API, a production reference repository with 25+ call sites) need a coordinated version bump with a mechanical `RawMessage`→`Message` rename.

**Benefits:**
- One concrete message type; middleware written as `message.Middleware` composes everywhere regardless of a given handler's marshaling configuration, closing the gap #125 identified.
- `Raw()` is a single, explicit boundary check instead of four incompatible generic instantiations standing in for the same concept.
- `ErrDataNotRaw`/`ErrDataNotTyped` catch a real bug class (e.g., a handler returning `[]byte` directly getting silently double-encoded) that the old compile-time-only guarantee didn't actually prevent once messages crossed pipe boundaries.

**Drawbacks:**
- Raw-vs-typed state is now a runtime invariant instead of a compile-time one; misuse surfaces as an `ErrDataNotRaw`/`ErrDataNotTyped` at run time instead of a build failure.
- `Data any` requires a type assertion (or `Raw()`) everywhere `Message` is consumed, where `RawMessage.Data []byte` previously did not.

## Links

- Supersedes: [ADR 0010](0010-dual-message-types.md) (Dual Message Types)
- Related: [#148](https://github.com/fxsml/gopipe/issues/148) (tracking issue), [#125](https://github.com/fxsml/gopipe/issues/125) (original proposal), [ADR 0032](0032-remove-message-engine.md) (prerequisite removal of `Engine`), [ADR 0031](0031-handler-level-marshaling.md) (Phase 2 — handler-level marshaling, builds on this ADR), `docs/plans/marshaling-strategy.md` (full design record)
