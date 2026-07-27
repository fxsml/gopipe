# Plan: Raw-by-Default Message Contract (Router)

**Status:** Proposed (design settled — no implementation yet)
**Related Issue:** [#125](https://github.com/fxsml/gopipe/issues/125) — Consider unifying Message and RawMessage for middleware composability
**Related ADRs:** [0010](../adr/0010-dual-message-types.md) (Dual Message Types — to be superseded), [0022](../adr/0022-message-package-redesign.md) (Message Package Redesign)
**Related Plans:** [archive/0008-marshal-unmarshal-pipes.decisions.md](archive/0008-marshal-unmarshal-pipes.decisions.md), [validation-marshaling-separation.decisions.md](validation-marshaling-separation.decisions.md)
**Depends On:** [engine-removal.md](engine-removal.md) — must complete before this plan's Phase 1 (dropping `TypedMessage[T]`/`RawMessage` would otherwise break `Engine`'s compile)
**Tracking Issues:** Phase 1 — [fxsml/gopipe#148](https://github.com/fxsml/gopipe/issues/148); Phase 2 — [fxsml/gopipe#149](https://github.com/fxsml/gopipe/issues/149)
**Related Bug (independent, not part of this plan):** [fxsml/gopipe#151](https://github.com/fxsml/gopipe/issues/151) — `Router` errors from `Use()` middleware are silently swallowed; see Final Design §6

## Overview

**Scope cut:** `Engine` is out of scope for this plan — but not because it's being left alone. It's tracked and removed entirely in its own plan, [engine-removal.md](engine-removal.md) ("Phase 0"), which must land first. `Router`, `Merger`, and `Distributor` survive as independent components and are what this plan actually touches.

The design adopts the proposal from #125 directly rather than a narrower slice of it: `Message` carries `Data any` and is assumed to hold raw bytes by default. A `Raw()` accessor detects whether conversion is needed. `Router` performs unmarshal/marshal inline, in the same worker goroutine that runs the handler, via a public `MarshalMiddleware(registry, marshaler) Middleware` that `Router` installs by default (configured via `Marshaler`, default JSON) and can be disabled with a single `DisableMarshaler` flag when a pipeline wants to build its own path.

**Fail loudly on mismatch, not silently both.** `Raw()` is used to *assert* the expected state, not to silently branch between accepting raw or typed input. Collapsing `Message`/`RawMessage` into one type (per #125) trades a compile-time guarantee (today, `UnmarshalPipe` takes `<-chan *RawMessage` — the compiler forbids feeding it typed data) for a runtime one; the whole point of paying that cost is to enforce it, not to quietly tolerate either state. See §2 and §3.

**Phasing:** `UnmarshalPipe`/`MarshalPipe` are in production use and stay — but they aren't left untouched. They get ported to the new `Message`-only contract (`*Message` in, `*Message` out, converting via `Raw()` internally, same as `Router` will) as the **first** implementation step, ahead of `Router`'s own built-in default behavior. Once ported, they also become the mechanism for opting out of `Router`'s inline default: compose them as explicit stages instead of relying on `Router`'s built-in conversion. `Router`'s built-in inline marshal/unmarshal (below) is a **second, later** phase.

No implementation has started. This document records the research, the design as it evolved through several rejected intermediate options, and the final shape for `Router` and the existing pipes.

## Rebased onto develop

Rebased onto `origin/develop` after ~15 commits landed there since this branch was created; replayed cleanly, no conflicts. Re-checked this plan's design against what changed:

- **`PipeConfig` gained `Metrics`, `Labels`, `LabelFunc`** (pipeline observability, unrelated feature area). Purely additive — coexists on the same struct as this plan's new `Marshaler`/`DisableMarshaler` fields, no conflict.
- **`Router` gained `Stats()`**, backed by an internal `atomic.Pointer[pipe.ProcessPipe[...]]` set in `Pipe()`. `UnmarshalPipe`/`MarshalPipe` gained the same `Stats()` plus Metrics/Labels wiring. Phase 1's pipe port and Phase 2's `Router` changes need to preserve this — additive, not a redesign.
- **`commandHandler.Handle()` now returns `ErrCommandDataMismatch`** instead of silently zero-valuing when `msg.Data` doesn't type-assert to `*C`/`C` (fixes #145). This reinforces this plan rather than conflicting with it: `MarshalMiddleware`'s unmarshal path already produces exactly `*C` via `registry.NewInput()`, so it's unaffected there; and for `DisableMarshaler: true` pipelines, a caller handing `Router` mistyped `Data` now gets a clear error instead of silent corruption — a net improvement for this plan's opt-out path.
- **`message/http` gained `SubscriberConfig.ErrorHandler`** for nack response handling (#140). Doesn't touch `*RawMessage`'s type signature — still out of this plan's scope.
- **New `message/otel` subpackage** — wraps `pipe.Metrics`/`Stats()` only, no coupling to `Message`/`RawMessage`. Unaffected.
- **ADR 0030 (drop `Merger`/`Distributor`/`Matcher`, decided via #153)** — removes `message.Merger`, `message.Distributor`, `pipe.Merger`, `pipe.Distributor`, `Matcher`, and `message/match` entirely. Final Design §3's mixed-raw/typed-sources recipe assumed `Merger` "survives" and used it to merge unmarshaled sources before feeding `Router` — that assumption is now wrong; corrected in place below to use `channel.Merge` instead. Not otherwise a conflict: this plan never depended on `Merger`/`Distributor` for anything besides that one recipe.
- **ADR 0031 (handler-level marshaling, supersedes this plan's own Final Design §3–5)** — moves marshal/unmarshal from a `Router`-installed `MarshalMiddleware`/`DisableMarshaler` (as designed below) to `CommandHandlerConfig`. §3–5 below are retained for historical context — the fail-loud rationale in §1–2 still applies — but should not be implemented as literally written; see ADR 0031 and #149 for the current authoritative Phase 2 design. §3's `channel.Merge` correction (previous bullet) still applies to the general shape of the opt-out recipe, just with `DisableMarshaler` now read as "per-handler" rather than "per-`Router`."

Design conflicts found: two (both noted above and corrected in place). Otherwise the design below stands as against current `develop`.

## Current State (Research)

### Issue and repo survey

- Issue [#125](https://github.com/fxsml/gopipe/issues/125) is open, unassigned, no linked PRs or comments. It proposes collapsing `*Message`/`*RawMessage` into one type (`Data any`, plus a `Raw() ([]byte, bool)` helper) so `Middleware[*Message, *Message]` works everywhere instead of four incompatible signatures (Proxy, Unmarshal, Marshal, Handler). It explicitly defers a decision pending impact evaluation — nothing was prototyped. This plan now adopts that proposal directly (see Final Design).
- No other open branch or PR touches marshal/unmarshal architecture.

### Prior art already in the repo

- **ADR 0010** (Dual Message Types) — `Message = TypedMessage[any]`, `RawMessage = TypedMessage[[]byte]` via type alias; zero runtime cost, same underlying struct. Superseded by this plan (see Consequences).
- **ADR 0022** (Message Package Redesign) — deliberately separated `Marshaler` (pure serialization) from `Handler` (self-describing, creates typed instances via `NewInput()`), and from `Router`/`Engine` orchestration. That separation is preserved — `Marshaler` stays a pure serialization interface; `Router` just calls it inline instead of via a dedicated pipe.
- **`archive/0008-marshal-unmarshal-pipes.decisions.md`** — design history for `UnmarshalPipe`/`MarshalPipe` as standalone public pipes, justified for reuse *outside* Engine. These are in production use today; this plan keeps and ports them rather than dropping them (see Final Design and Consequences).
- **`validation-marshaling-separation.decisions.md`** — resolved in favor of pure separation: `jsonschema.Registry` + middleware validates raw bytes without requiring Go types, independent of the `Marshaler`. Untouched by this plan — that middleware operates purely on bytes and never needs `Router` involved.

### Current marshal/unmarshal topology (being replaced, Router path only)

`Router` today assumes its input channel already carries typed `*Message` — conversion from `*RawMessage` happens upstream via a dedicated `UnmarshalPipe`, and conversion back happens downstream via a dedicated `MarshalPipe` (see `message/pipes.go`), each with its own worker pool and buffered channel. This plan removes the need for a *separate pipe stage* to do that conversion — `Router` does it inline by default. It does not mean `Router` tolerates a mix of raw and typed messages on the same channel: in default mode every message is expected raw, full stop (see "Fail loudly," above, and §3).

## Final Design

### 1. `Message` becomes a concrete struct — `TypedMessage[T]` is dropped

```go
type Message struct {
    Data       any
    Attributes Attributes
    acking     *Acking
    locals     map[any]any
}

// Raw reports whether Data currently holds raw bytes, and returns them.
func (m *Message) Raw() ([]byte, bool) {
    b, ok := m.Data.([]byte)
    return b, ok
}
```

No generic parameter, no `RawMessage` alias, no `NewTyped[T]`. `New(data any, attrs, acking)` is the only constructor. `Copy(msg *Message, data any) *Message` loses its generic type parameters. This is a deliberate, larger scope cut than initially planned — it gives up the "compile-time-typed pipeline" use case ADR 0010 introduced `TypedMessage[T]` for for the sake of one simple, uniform type.

### 2. `UnmarshalPipe`/`MarshalPipe` port to `*Message` → `*Message` (Phase 1, implemented first)

These stay as public pipes — they're in production — but their signature changes from `pipe.ProcessPipe[*RawMessage, *Message]` / `pipe.ProcessPipe[*Message, *RawMessage]` to a single uniform `pipe.ProcessPipe[*Message, *Message]`, using `Raw()` internally exactly like `Router` will:

```go
func NewUnmarshalPipe(registry InputRegistry, marshaler Marshaler, cfg PipeConfig) *UnmarshalPipe {
    p := &UnmarshalPipe{}
    p.inner = pipe.NewProcessPipe(func(ctx context.Context, msg *Message) ([]*Message, error) {
        raw, ok := msg.Raw()
        if !ok {
            return nil, fmt.Errorf("%w: got %T", ErrDataNotRaw, msg.Data)
        }
        instance := registry.NewInput(msg.Type())
        if instance == nil {
            return nil, ErrUnknownType
        }
        if err := marshaler.Unmarshal(raw, instance); err != nil {
            return nil, err
        }
        msg.Data = instance
        return []*Message{msg}, nil
    }, ...)
    return p
}

func (p *UnmarshalPipe) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    return p.inner.Pipe(ctx, in)
}
```

`MarshalPipe` is symmetric: `*Message` in, `*Message` out, and fails with `ErrDataNotTyped` if `Data` is *already* `[]byte` instead of silently passing it through — a message reaching `MarshalPipe` already raw is a wiring bug (something upstream marshaled it, or the wrong channel feeds this stage), not a legitimate shortcut. Behavior (unmarshal/marshal semantics, error handling, logging) is otherwise preserved from today — what changes is the channel type (`<-chan *RawMessage` → `<-chan *Message`), the new fail-loud checks, and, as a result, the middleware type they accept: both pipes can now use the same `message.Middleware` type `Router` uses, instead of needing generic `pipe/middleware.Middleware[In, Out]` at all. **This is a breaking change to these two pipes' public signatures** (not just their behavior, now — the fail-loud checks are new/stricter behavior too) — it's the one deliberate break this plan makes to production-used API, done once, up front, so everything downstream (including opting out of `Router`'s default) speaks the same `*Message` type.

With this in place, opting out of `Router`'s built-in default (Phase 2 below) is simply: don't rely on it — compose `UnmarshalPipe`/`Router`/`MarshalPipe` explicitly as separate stages instead, same as today's pattern, just uniformly typed.

### 3. `Router`: marshal/unmarshal as a public, swappable `Middleware` (Phase 2, later)

The built-in conversion is **not** baked directly into `Router.process()`. It's an ordinary, public `Middleware` — `Router` just happens to install it by default. This is what makes it swappable instead of merely togglable (see §5, needed for `jsonschema`):

```go
var (
    // ErrDataNotRaw is returned when a message reaches a conversion stage
    // (default-mode Router, UnmarshalPipe) with Data that isn't raw []byte.
    ErrDataNotRaw = errors.New("expected raw []byte data")

    // ErrDataNotTyped is returned when handler/next output already holds
    // raw []byte Data at a point where marshaling is about to be applied
    // (default-mode Router, MarshalPipe) — marshaling it again would silently
    // double-encode.
    ErrDataNotTyped = errors.New("unexpected raw []byte data, want typed")
)

// MarshalMiddleware returns middleware that unmarshals raw Data into a typed
// instance (via registry.NewInput) before calling next, and marshals typed
// Data returned by next back into raw bytes.
//
// This asserts the expected state rather than tolerating either: input must
// be raw (ErrDataNotRaw otherwise), and output must not already be raw
// (ErrDataNotTyped otherwise) — a mismatch means something upstream
// already converted the message, or the wrong channel/handler is wired in.
func MarshalMiddleware(registry InputRegistry, marshaler Marshaler) Middleware {
    return func(next ProcessFunc) ProcessFunc {
        return func(ctx context.Context, msg *Message) ([]*Message, error) {
            raw, ok := msg.Raw()
            if !ok {
                return nil, fmt.Errorf("%w: got %T", ErrDataNotRaw, msg.Data)
            }
            instance := registry.NewInput(msg.Type())
            if instance == nil {
                return nil, ErrUnknownType
            }
            if err := marshaler.Unmarshal(raw, instance); err != nil {
                return nil, fmt.Errorf("unmarshal: %w", err)
            }
            msg.Data = instance

            outputs, err := next(ctx, msg)
            if err != nil {
                return nil, err
            }

            for _, out := range outputs {
                if _, ok := out.Raw(); ok {
                    return nil, ErrDataNotTyped
                }
                data, err := marshaler.Marshal(out.Data)
                if err != nil {
                    return nil, fmt.Errorf("marshal: %w", err)
                }
                out.Data = data
                if out.Attributes == nil {
                    out.Attributes = make(Attributes)
                }
                out.Attributes[AttrDataContentType] = marshaler.DataContentType()
            }
            return outputs, nil
        }
    }
}
```

This closes a real gap the earlier sketch had: it only checked `Raw()` on the *input* side, and unconditionally called `marshaler.Marshal(out.Data)` on output with no check at all. A handler returning `[]byte` as its event type directly (`NewCommandHandler[Cmd, []byte]` is valid Go) would have silently base64-encoded already-raw output via `JSONMarshaler` — a real double-encoding bug, not just a hypothetical. The output-side `Raw()` check fixes that at the same time as adding the loud-failure behavior.

```go
type PipeConfig struct {
    ...
    Marshaler        Marshaler // default: NewJSONMarshaler() — single combined interface, unchanged from today
    DisableMarshaler bool      // when true, Router installs no built-in conversion middleware — build the path yourself via Use()
}
```

`r.process()` goes back to being simple — handler lookup, matcher check, `Handle()` — no conversion logic at all, always assumes typed `Data`:

```go
func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
    entry, ok := r.handler(msg.Type())
    if !ok {
        return nil, ErrNoHandler
    }
    if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
        return nil, ErrHandlerRejected
    }
    return entry.handler.Handle(msg.Context(ctx), msg)
}
```

`Router.Pipe()` installs `MarshalMiddleware(r, r.cfg.Marshaler)` — `r` itself as the `InputRegistry`, unchanged — as the innermost wrapper, unless `DisableMarshaler` is set:

```go
fn := r.process
if !r.cfg.DisableMarshaler {
    fn = MarshalMiddleware(r, r.cfg.Marshaler)(fn)
}
for i := len(r.middleware) - 1; i >= 0; i-- {
    fn = r.middleware[i](fn)
}
fn = r.ackingMiddleware()(fn)
```

One switch covers both directions symmetrically: `DisableMarshaler` means "`Router` never touches `Data`, at all" — not an asymmetric per-direction gate. Anyone needing the built-in behavior but with different validation/ordering semantics (see §5) sets `DisableMarshaler: true` and registers their own middleware, built from the same `Marshaler` primitive, via `Use()`.

The `DisableMarshaler` field's godoc should state the general rule, not just point at `Subject()` as a one-off case: *any* `Use()` middleware that reads or sets `msg.Data`/`out.Data` expecting a concrete Go type — not `Attributes` — needs `DisableMarshaler: true`, because `Use()` middleware always runs outside the built-in conversion (pre-unmarshal input, post-marshal output; see §5). `Subject()` is documented as the one existing example, cross-referenced from both sides, so the constraint is discoverable from `Router`'s docs by anyone writing new middleware, not only by someone already suspicious of `Subject()`.

**Consequence of fail-loud: no implicit mixed raw/typed sources.** `DisableMarshaler` (as designed in this section — see the note above pointing to [ADR 0031](../adr/0031-handler-level-marshaling.md) for where this setting actually ended up living) is not a per-message switch — there is no longer a way to silently accept some messages raw and others already typed on the same channel without an explicit unmarshal step. Today's `Engine` (being removed regardless — see [engine-removal.md](engine-removal.md)) supported exactly that: `AddInput` (typed) and `AddRawInput` (raw, via `UnmarshalPipe`) fed the same shared `Merger`, which fed one `Router`. Reproducing that pattern now requires explicit composition: unmarshal the raw sources first (via the ported `UnmarshalPipe`), merge everything as uniformly typed (via **`channel.Merge`, not `message.Merger`** — the latter is removed under [ADR 0030](../adr/0030-drop-merger-distributor-matcher.md)/#153 — and `channel.Merge` covers this recipe's needs since it only ever merges sources known upfront, not dynamically added later), and feed the result to handlers configured with `DisableMarshaler: true`. This is a deliberate constraint, not an oversight: it's what makes the fail-loud behavior meaningful instead of best-effort.

### 4. Middleware signature collapses to one

Today: `Middleware[*RawMessage,*Message]` (unmarshal), `Middleware[*Message,*RawMessage]` (marshal), `Middleware[*Message,*Message]` (handlers), `Middleware[*RawMessage,*RawMessage]` (proxy) — four incompatible instantiations, per #125. After: everything is `message.Middleware = func(ProcessFunc) ProcessFunc` over `*Message` — the type `Router` already uses, and the type `MarshalMiddleware` itself returns. Validation, correlation-ID, deadline, and any future middleware compose via the same `Use()`, regardless of whether the message happens to be raw or typed at the point they run — though *which* it is at that point is not arbitrary; see §5.

### 5. Provided middleware — impact analysis

Collapsing to one middleware type doesn't mean every existing middleware is unaffected — whether a message is raw or typed at the point a given `Use()` middleware runs depends on where the built-in `MarshalMiddleware` sits (innermost, per §3): **`Use()` middleware always sees pre-unmarshal input and post-marshal output**, because the built-in conversion is nested *inside* the `Use()` chain, not outside it.

**`message/jsonschema`** (`NewValidationMiddleware`, `NewInputValidationMiddleware`, `NewOutputValidationMiddleware`) — this positioning is exactly what they need: input validation wants raw bytes *before* unmarshal, output validation wants raw bytes *after* marshal. Both naturally line up with `Use()`'s vantage point. **Scope kept minimal, as instructed** — no redesign, no collapsing into one middleware, no `Registry` changes. The only change is mechanical: retype from `middleware.Middleware[*message.RawMessage, ...]` (generic `pipe/middleware`) to `message.Middleware` over `*message.Message`, and swap direct `.Data` access for `msg.Raw()`. Same three functions, same call sites, same behavior.

**`message/middleware`** — checked all five, re-verified by grep for `.Data` across the actual middleware source (not just re-reading):
- `CorrelationID()`, `Deadline()`, `Recover()`, `ValidateRequired()` — **zero** `.Data` references in any of the four files. They operate only on `Attributes` (or nothing at all, for `Recover()`). **Unaffected**, raw or typed makes no difference.
- **`Subject()` — the single exception, and it breaks silently under `Router`'s default (marshal-enabled) mode.** It does `if s, ok := out.Data.(subjecter); ok { ... }` on `next()`'s output — a duck-typed check that only succeeds if `Data` is still a concrete Go type. But by the time `next()` returns to a `Use()`-registered middleware, the built-in `MarshalMiddleware` (nested inside) has already marshaled `out.Data` to `[]byte`. The type assertion always fails, `AttrSubject` never gets set — no error, just a silent no-op. This is a real regression, not a rename.

So the finding is a single root cause with a single concrete instance today (`Subject()`), not a pattern spread across several middleware — but the constraint it exposes is general (any middleware reading/setting typed `Data`), so it's documented in two places: a specific note on `Subject()`'s godoc, and the general rule on `Router`'s `DisableMarshaler` godoc (§3) so it's discoverable independent of already knowing about `Subject()`.

**Root cause:** `jsonschema`'s middleware and `Subject()` want opposite things at the same seam. `jsonschema` wants to sit *outside* the conversion boundary (raw in, raw out) — satisfied by the innermost built-in positioning. `Subject()` wants to sit *inside* it (typed only, between `Handle()` and marshal) — which no position of a single combined `Use()`-relative built-in middleware can satisfy simultaneously with `jsonschema`'s requirement. There is no single placement that serves both.

**Resolution — no new API, per "don't scope creep":** `Subject()` requires `DisableMarshaler: true`. On a `Router` with the built-in conversion disabled, `Data` stays typed all the way through `Use()` middleware (in and out), so `Subject()` works exactly as it does today — marshaling, if needed, happens downstream as an explicit step (e.g. the ported `MarshalPipe` from §2, composed after `Router`), which recreates the seam `Subject()` needs via explicit pipe composition instead of implicit middleware ordering. This needs a godoc update in **two places**, not one: `Subject()` itself, and `Router`'s `DisableMarshaler` field (§3) stating the general rule so future middleware authors find it without already knowing `Subject()` is affected. No Router redesign, no second middleware hook.

### 6. Error logging: move to the real boundary, not just add more inline calls

While reviewing where `MarshalMiddleware`'s new `ErrDataNotRaw`/`ErrDataNotTyped` errors should be logged, a pre-existing gap surfaced in `Router` itself — **tracked as its own bug, independent of this plan: [fxsml/gopipe#151](https://github.com/fxsml/gopipe/issues/151).** Not fixing it here would mean the new errors inherit the same gap.

The gap: `pipe.Config.ErrorHandler`'s own godoc says "Default logs via slog.Error" — the `pipe` package would log every error centrally if left alone. But `Router.Pipe()` overrides it:

```go
ErrorHandler: func(in any, err error) {
    msg := in.(*Message)
    msg.Nack(err)
    r.cfg.ErrorHandler(msg, err) // defaults to a no-op
},
```

This suppresses the pipe package's default logging and replaces it with Nack-only. Meanwhile `process()` separately hand-rolls logging, but only inline at its own three failure sites (`ErrNoHandler`, `ErrHandlerRejected`, handler execution failure). Any error returned by a `Use()`-registered middleware (`ValidateRequired`, `Deadline`, `jsonschema`'s validation middleware, `Subject()`) never reaches `process()` at all — middleware short-circuits before calling `next()` — so it hits the overridden `ErrorHandler`, which doesn't log. **By default, those errors are completely silent today:** Nacked, no log line, unless the caller supplies their own `ErrorHandler` that happens to log.

**Decision for this plan:** rather than adding a fourth inline `Logger.Error` call inside `MarshalMiddleware` (which would still leave the underlying gap in place for every other middleware), move logging to the actual boundary — the `pipe.Config.ErrorHandler` closure in `Router.Pipe()`, which structurally sees every error from the whole chain (middleware, `MarshalMiddleware`, and `process()` alike) exactly once:

```go
cfg := pipe.Config{
    ...
    ErrorHandler: func(in any, err error) {
        msg := in.(*Message)
        msg.Nack(err)
        r.cfg.Logger.Error("Processing failed",
            "component", "router",
            "error", err,
            "attributes", msg.Attributes)
        r.cfg.ErrorHandler(msg, err)
    },
}
```

`process()`'s three inline `Logger.Error` calls come out entirely — it just returns errors. `MarshalMiddleware` needs no `Logger` dependency of its own; its errors are covered by this same central point, for free, keeping it minimal.

**Trade-off:** loses the differentiated message text per failure site ("Routing message failed" vs. "Matching handler failed" vs. "Executing handler failed") in favor of one generic line — though the sentinel errors' own text (`"no handler for message type"`, `"message rejected by handler matcher"`) is descriptive enough to grep on. Since `Router` is in production, if anyone's alerting matches the old specific outer message strings rather than the error text, that's a real behavior change worth calling out in the CHANGELOG, not just folding in silently.

This fix is filed and land-able independently of this plan (it's a `Router` bug today, with or without the marshaling redesign) but is a prerequisite in practice for §3's new errors to be observable by default — sequence it alongside or before Phase 2.

## Design Evolution — Rejected Intermediate Options

Recorded here because they were seriously considered before landing on the Final Design above.

### Rejected: marshal/unmarshal as `Engine`-installed middleware, `RawMessage` kept as a distinct type

Earlier direction: keep the `TypedMessage[T]` dual-type system, keep `RawMessage`, and make `Engine` install unmarshal/marshal as ordinary `Middleware[*Message,*Message]` around `Router`'s existing worker, converting `*RawMessage` → `*Message` at ingress as a free struct copy. This was a narrower, lower-risk slice of #125.

**Why rejected:** `Engine` was cut from v1 scope entirely, which removed the motivating use case (Engine's per-input/output dedicated pipe stages and their shutdown-cascade complexity). Once `Router` is the only thing in scope, there's no reason to route the conversion through a middleware layer instead of building it directly into `Router.process()` — and no reason to keep two type instantiations (`Message`/`RawMessage`) of the same struct when the boundary check (`Raw()`) already tells you everything you need to know.

### Rejected: `NoOpUnmarshaler`/`NoOpMarshaler` that copy into `*[]byte`

First opt-out attempt: split `Marshaler` into `Unmarshaler`/`Marshaler` interfaces, and provide no-op implementations that still did work — `NoOpUnmarshaler.Unmarshal` type-asserted the target to `*[]byte` and copied raw bytes into it, requiring `Handler.NewInput()` to return `*[]byte` for opted-out handlers.

**Why rejected:** This wasn't actually "opting out" — it still required every handler using it to be specially shaped, and it conflated "skip conversion" with "convert into a specific alternate shape." The real requirement was simpler: when typed data is wanted, do *literally nothing* to `Data`.

### Rejected: `NoOpUnmarshaler`/`NoOpMarshaler` as pure sentinel types

Second attempt: keep the no-op types, but make their methods true no-ops (`return nil`), and have `Router` type-switch on the configured `Marshaler`/`Unmarshaler` to skip the conversion block entirely when a no-op sentinel is configured.

**Why rejected:** The dead method bodies were a footgun — if the sentinel type were ever called directly (e.g., someone using it standalone), it would silently misbehave rather than error. It also required two new exported types and a split interface for something that, on inspection, decomposes into one gate that already exists for free (`Raw()`, for unmarshal) and one gate that doesn't need a whole type to express (a bool, for marshal).

### Rejected: splitting `Marshaler` into `Unmarshaler`/`Marshaler` interfaces

Considered alongside the no-op sentinel attempts, to let each direction be configured independently.

**Why rejected:** Once the no-op sentinels were dropped in favor of `Raw()` (structural, for unmarshal) + `DisableMarshaling bool` (explicit, for marshal), there was no remaining requirement driving the split — no concrete use case for plugging in different implementations per direction. Keeping the single combined `Marshaler` interface (unchanged from today) is less API surface for the same behavior.

## Consequences / Ripple Effects

| Area | Impact |
|---|---|
| `TypedMessage[T]`, `RawMessage`, `NewTyped[T]`, `NewRaw` | Removed. `Message` is a concrete struct; `New(data any, attrs, acking)` is the only constructor. |
| Generic `Copy[In, Out any]` | Becomes `Copy(msg *Message, data any) *Message`. |
| `ADR 0010` (Dual Message Types) | Needs a new ADR marking it Superseded. |
| `ParseRaw`/`parseRawBytes` | Return `*Message` with `Data []byte` instead of `*RawMessage`. Public API change. |
| `message/pipes.go` (`UnmarshalPipe`/`MarshalPipe`) | **Kept, ported first.** Signature changes from `*RawMessage`↔`*Message` to uniform `*Message`→`*Message`, plus new fail-loud checks (behavior is *not* fully preserved — see `ErrDataNotRaw`/`ErrDataNotTyped` row below). This is the first implementation step, ahead of `Router`'s own inline behavior, and becomes the supported way to opt out of `Router`'s default. |
| `Handler` interface | Unchanged. |
| `InputRegistry` / `Router.NewInput()` | Unchanged — still how `MarshalMiddleware` gets an instance to unmarshal into. |
| Error handling | Unmarshal/marshal errors flow through the same auto-nack + `ErrorHandler` path as today. New sentinel errors (`ErrDataNotRaw`, `ErrDataNotTyped`) are added, but the path they flow through is unchanged. |
| `ErrDataNotRaw` / `ErrDataNotTyped` (new) | Default-mode `Router` (`MarshalMiddleware`) and `UnmarshalPipe`/`MarshalPipe` now fail loudly instead of silently accepting whatever state `Data` is in: input must be raw, output-to-marshal must not already be raw. Closes a real bug in the untested prior sketch — `MarshalMiddleware` never checked `Raw()` on output at all, so a handler returning `[]byte` directly would have been silently base64-encoded by `JSONMarshaler`. Also removes the implicit "mix raw and typed messages on one channel" pattern `Engine` used to support (see Final Design §3). |
| `Router` error logging (independent bug, [#151](https://github.com/fxsml/gopipe/issues/151)) | `process()`'s three inline `Logger.Error` calls are removed; logging moves to the `pipe.Config.ErrorHandler` closure in `Router.Pipe()`, the actual boundary that sees every error (middleware + `MarshalMiddleware` + `process()`). Fixes a pre-existing gap — `Use()`-middleware errors were previously silent by default. Filed and fixable independently of this plan, but a practical prerequisite for the new errors above to be observable (see Final Design §6). |
| `DisableMarshaler` mode | No new checks added here — `Router` doesn't inspect `Data` at all in this mode by design. Mis-shaped `Data` reaching a `commandHandler`-based handler is already caught by the existing `ErrCommandDataMismatch` (fixes #145, already on `develop`); `NewHandler`-based handlers remain the caller's own responsibility, consistent with "Handler is self-describing." |
| `message/jsonschema` middleware | Mechanical retype only (`message.Middleware` / `*message.Message` / `msg.Raw()`). No logic or API shape change — see Final Design §5. |
| `message/middleware.Subject()` | **Breaking behavior change.** Silently stops setting `AttrSubject` when used on a `Router` with the default (marshal-enabled) `MarshalMiddleware` installed. Requires `DisableMarshaler: true` going forward. Needs a godoc update on `Subject()` *and* on `Router.DisableMarshaler` (general rule, not a `Subject()`-specific caveat). |
| `message/middleware` (`CorrelationID`, `Deadline`, `Recover`, `ValidateRequired`) | Unaffected — `Attributes`-only, never touch `Data`. |

## Open Questions

1. ~~Keep `UnmarshalPipe`/`MarshalPipe` at all for v1?~~ **Resolved: keep both, port them to `*Message`→`*Message` as Phase 1** (see Final Design §2). They stay as production API and become the supported opt-out path for `Router`'s Phase 2 default behavior.
2. **`DataContentType` validation on unmarshal** — should `Router` check the incoming `datacontenttype` attribute against the configured `Marshaler` before unmarshaling, or stay silent like today? Proposed: stay silent for v1, no new behavior.
3. **ADR required.** This changes an architectural pattern and removes a public type (`TypedMessage[T]`/`RawMessage`) per `docs/procedures/adr.md`. An ADR superseding ADR 0010 is needed before implementation — it should cover both phases (pipes port + Router default) since they share the same underlying `Message` contract change.

## Benchmark Plan

The original motivation for benchmarking — comparing Engine's dedicated-pool pipe stages against an inline alternative — no longer applies now that Engine is out of scope; there is no competing "before" implementation to compare against. What remains useful is characterizing `Router`'s own inline marshal/unmarshal cost before merging, using the same scenario shapes considered earlier:

1. **Small payload, high volume, negligible handler cost** — baseline per-message overhead of the inline unmarshal/marshal calls.
2. **Large/nested payload (few KB), cheap handler** — CPU-bound marshal cost sharing the router's worker pool with handler execution; run at multiple `Concurrency` settings to see whether marshal cost becomes a bottleneck at low concurrency.
3. **Realistic I/O-bound handler (simulated latency, e.g. 100µs–1ms sleep), moderate concurrency** — confirms marshal cost hides behind I/O wait in the common case.

Metrics: ns/op, B/op, allocs/op (`testing.B`), plus `DisableMarshaler` on/off comparison to quantify the marshal step's own cost in isolation.

## Next Steps

- [ ] Write ADR superseding ADR 0010 (drop `TypedMessage[T]`/`RawMessage`, adopt raw-by-default `Message` contract; covers both phases)
- [ ] Implement `Message` struct simplification (drop generic, add `Raw()`)

**Phase 1 — port existing pipes (do this first):**
- [ ] Add `ErrDataNotRaw`/`ErrDataNotTyped` to `message/errors.go`
- [ ] Port `UnmarshalPipe`/`MarshalPipe` to `*Message` → `*Message`, failing loudly on mismatch rather than silently passing through (Final Design §2)
- [ ] Update pipe middleware usage to `message.Middleware`
- [ ] Update CHANGELOG (breaking change to these two pipes' signatures *and* behavior — new fail-loud checks, not just a retype)

**Phase 2 — Router built-in default (later):**
- [ ] Fix [#151](https://github.com/fxsml/gopipe/issues/151) (move error logging to the `pipe.Config.ErrorHandler` boundary) — independent bug, sequence alongside or before the rest of Phase 2 so the new errors below are observable by default (Final Design §6)
- [ ] Implement public `MarshalMiddleware(registry, marshaler) Middleware` with fail-loud input/output checks (Final Design §3)
- [ ] Implement `Router` changes (`Marshaler`/`DisableMarshaler` config, install `MarshalMiddleware` by default, simplify `process()`)
- [ ] Retype `message/jsonschema`'s three middleware constructors (mechanical only — Final Design §5)
- [ ] Update `Subject()` godoc to document the `DisableMarshaler: true` requirement (Final Design §5)
- [ ] Update `Router.DisableMarshaler` field godoc with the general rule (any middleware reading/setting typed `Data` needs it), cross-referenced with `Subject()` — not discoverable only via `Subject()`'s docs (Final Design §3, §5)
- [ ] Add benchmarks (scenarios above)
- [ ] Update CHANGELOG (note `Subject()` behavior change and the fail-loud mismatch errors explicitly, not just as a signature change)
- [ ] Update this plan's status to Complete
