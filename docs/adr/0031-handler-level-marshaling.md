# ADR 0031: Marshaling Lives in CommandHandler, Not Router

**Date:** 2026-07-27
**Status:** Proposed

## Context

The original raw-by-default design (#149, `docs/plans/marshaling-strategy.md`) put automatic marshal/unmarshal in `Router` itself: a `MarshalMiddleware` installed by default, opt-out via a `DisableMarshaler` field. Evidence-gathering against a production reference repository found this created a real correctness hazard: `middleware.Subject()`, confirmed live in production, depends on typed output `Data` — but `Router`-level middleware can't reliably assume any particular marshal state once even one handler on that router self-marshals, and nothing enforced consistency across handlers registered at different times by different people. `Router` dispatches purely on the CE `type` attribute, which never required knowing the payload's Go type — that coupling was never architecturally necessary.

## Decision

### 1. `Router` stays pure dispatch; marshaling moves to `NewCommandHandler`

`Handler` drops `NewInput()`. `Router.process()` is lookup-by-type-then-`Handle()`, nothing else — no `Marshaler`/`DisableMarshaler` config, no installed middleware, no `InputRegistry` implementation.

```go
type Handler interface {
    EventType() string
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

`NewCommandHandler` gains the marshal behavior, on by default, opt-out per handler at construction time — not a router-wide flag, so the decision is visible at the exact call site it applies to, not liable to drift as unrelated handlers get added later:

```go
type CommandHandlerConfig struct {
    Source           string
    Naming           EventTypeNaming
    Attributes       Attributes
    Marshaler        Marshaler              // default NewJSONMarshaler()
    DisableMarshaler bool                   // default false
    Subject          func(data any) string  // optional, see decision 2
}
```

`NewHandler[T]` is unchanged — the fully manual escape hatch, never had a marshaling opinion.

`InputRegistry`/`FactoryMap`/`UnmarshalPipe`/`MarshalPipe` are unaffected — they remain the correct tool for explicit composition (external-naming unmarshal decoupled from dispatch, or a `Router` whose handlers use `DisableMarshaler: true` feeding further typed processing before an eventual explicit `Marshal` stage).

### 2. Shipped `message/middleware` may only touch `Attributes`/`locals`, never `Data`

This is what makes decision 1 safe without per-router consistency requirements: if no shipped middleware ever depends on `Data`'s concrete type, `Router`-level `.Use()` middleware is safe regardless of which handlers on that router marshal internally and which don't. `CorrelationID()`, `Deadline()`, `Recover()`, `ValidateRequired()` already satisfy this. `Subject()` — the one exception, and the one with the confirmed production dependency — is removed from `message/middleware` entirely, not merely gated behind documentation.

Its capability moves to `CommandHandlerConfig.Subject func(data any) string`, called inside `Handle()`'s own output-construction loop, before marshaling — guaranteed typed access, no timing dependency, no duck-typed interface requirement on domain types, and no silent no-op: the old `Subject()` type-asserted `Data.(subjecter)` and quietly did nothing if the type didn't match; a plain callback either runs or isn't configured.

## Consequences

**Breaking Changes:**
- `Handler.NewInput()` removed; `Router` no longer implements `InputRegistry`
- `message/middleware.Subject()` removed
- `CommandHandlerConfig` gains `Marshaler`, `DisableMarshaler`, `Subject`

**Benefits:**
- No `Router`-level correctness hazard — nothing to migrate silently-incorrectly, because there's no router-wide marshal default to be inconsistent with
- Less new API than the original design: no exported `MarshalMiddleware` function, no registry needed at the point marshaling happens (the handler already knows its own concrete input type)
- `Subject()`'s replacement is strictly safer than what it replaces, not just relocated

**Drawbacks:**
- `Router`-level middleware is now a smaller, more constrained toolset (`Attributes`/`locals` only) — anything needing typed `Data` must be handled per-handler via `CommandHandlerConfig` or a custom `Handler`, not as reusable shipped middleware

## Links

- Supersedes the `Router`-level design in [#149](https://github.com/fxsml/gopipe/issues/149) and `docs/plans/marshaling-strategy.md`'s Final Design §3–5
- Related: [#148](https://github.com/fxsml/gopipe/issues/148) (`Message`/`Raw()` — this decision builds on it), ADR 0028 (external dependency policy — same "proven, not just plausible" standard applied to `Subject()`'s removal)
