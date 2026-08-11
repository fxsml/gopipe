# ADR 0032: Remove Message Engine

**Date:** 2026-08-05
**Status:** Implemented

## Context

`message.Engine` (ADR 0020/0022) is an orchestration layer that wires a `Merger`, `Router`, and `Distributor` together with dedicated per-input/output unmarshal/marshal pipe stages, a `Plugin` abstraction, and its own shutdown-cascade machinery.

Checked against real-world evidence — a production reference repository (three services) and the `gopipe-azservicebus` broker adapter: zero usage of `Engine`, `EngineConfig`, or `Plugin` in either. This independently confirms `Engine` is unused orchestration infrastructure, not a latent dependency this removal would break. It also duplicates the same dedicated-pool marshal-stage pattern later identified as premature optimization in the raw-by-default `Router` redesign (#125).

`Engine` currently depends on `RawMessage`/`TypedMessage[T]`, which the [marshaling-strategy plan](../plans/marshaling-strategy.md) drops in its Phase 1. Removing `Engine` first avoids spending effort mechanically porting something already out of scope.

## Decision

Remove `Engine` and its `Plugin` abstraction entirely — no deprecation period, no compatibility shim (pre-v1, single consumer base).

Deleted:
- `message.Engine`, `message.EngineConfig`, `message.Plugin`, `message.ErrInputRejected` (`message/engine.go`, `message/engine_test.go`, `message/engine_bench_test.go`)
- `message/cloudevents.SubscriberPlugin`, `PublisherPlugin` (`message/cloudevents/plugin.go`, `plugin_test.go`) — pure `Engine`-wiring sugar

Kept, unaffected:
- `message.Router` — independent, general-purpose component, already zero-dependency on `RawMessage`/`TypedMessage[T]`
- `message/cloudevents.Subscriber`, `Publisher`, `adapter.go` — produce/consume raw channels directly, never `Engine`-dependent
- `message.Merger`/`Distributor` — their fate is decided separately by [ADR 0030](0030-drop-merger-distributor-matcher.md)/#153; out of scope here

Composition without `Engine`: a raw input channel feeds `message.NewUnmarshalPipe` → `Router.Pipe()` → `message.NewMarshalPipe` → a raw output channel. `Router.Use()` still applies middleware exactly as before; `Router`'s pool config, ack strategy, and logging all apply transparently.

## Consequences

**Breaking Changes:**
- Removes `message.Engine`, `message.EngineConfig`, `message.Plugin`, `message.ErrInputRejected`, `message/cloudevents.SubscriberPlugin`, `message/cloudevents.PublisherPlugin` (pre-v1, acceptable)
- `message/middleware/correlation_test.go`, `message/doc.go`, `message/README.md`, `examples/04-message`, `examples/06-http-cloudevents` rewritten to compose `Router` directly instead of `Engine`

**Benefits:**
- Removes unused orchestration infrastructure and its `Plugin` abstraction, consistent with ADR 0028 rule 4 ("proven, not just plausible, real-world use")
- Unblocks the marshaling-strategy plan's Phase 1 (dropping `TypedMessage[T]`/`RawMessage`), which `Engine` would otherwise block

**Drawbacks:**
- Consumers that want automatic merger/router/distributor wiring must compose it themselves from `Router` (+ `channel.Merge`/`channel.Switch` for fan-in/fan-out, per ADR 0030) — recoverable from git history if a real need for the automated wiring ever appears

## Links

- Supersedes: ADR 0020 (Message Engine Architecture)
- Related: ADR 0022 (Message Package Redesign) — see its Updates note
- Related: ADR 0028 (External Dependency Policy, rule 4), ADR 0030 (Drop Merger, Distributor, and Matcher)
- Tracking issue: [fxsml/gopipe#147](https://github.com/fxsml/gopipe/issues/147)
- Plan: [engine-removal.md](../plans/engine-removal.md)
