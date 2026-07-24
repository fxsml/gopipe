# Plan: Remove Message Engine

**Status:** Proposed
**Related ADRs:** [0020](../adr/0020-message-engine-architecture.md) (Message Engine Architecture — to be superseded), [0022](../adr/0022-message-package-redesign.md) (Message Package Redesign — needs an Updates note)
**Depended On By:** [marshaling-strategy.md](marshaling-strategy.md) (its Phase 1 requires this plan complete first)
**Tracking Issue:** [fxsml/gopipe#147](https://github.com/fxsml/gopipe/issues/147)

## Overview

`Engine` is removed entirely, not kept and mechanically ported. It's the orchestration layer introduced by ADR 0020/0022 — a merger/router/distributor assembly with dedicated per-input/output unmarshal/marshal pipe stages, config-based `AddInput`/`AddRawInput`/`AddOutput`/`AddRawOutput`/`AddPlugin`, and its own shutdown-cascade machinery. In practice it's legacy overengineering: infrastructure built ahead of a real need, adding a whole orchestration layer, a `Plugin` abstraction, and dedicated-pool marshal stages that [marshaling-strategy.md](marshaling-strategy.md) independently concluded were premature.

This plan pays off directly: `Engine` currently depends on `RawMessage`/`TypedMessage[T]`, which [marshaling-strategy.md](marshaling-strategy.md) drops. Rather than spending effort mechanically retyping `Engine` just to keep something already out of scope compiling, this plan removes it outright — which is also why it must land **before** that plan's Phase 1 (dropping `TypedMessage[T]`/`RawMessage` would otherwise break `Engine`'s compile).

`Router`, `Merger`, and `Distributor` are not going anywhere — they're kept as independent, general-purpose components usable without `Engine`. Only the orchestration layer that wired them together automatically is removed.

## Goals

1. Delete `Engine` and its `Plugin` abstraction entirely — no deprecation period, no compatibility shim (pre-v1, single consumer base, consistent with the project's existing "pre-v1, breaking changes acceptable" posture).
2. Confirm `Merger`/`Distributor` survive unmodified as standalone components (already verified: zero `RawMessage`/`TypedMessage` dependency in either).
3. Leave no dangling references — every test, example, and doc that wires through `Engine` gets rewritten to use `Router` (+ `Merger`/`Distributor` directly where fan-in/fan-out is actually needed), not just deleted.

## Tasks

### Task 1: Delete Engine core

**Files to delete:**
- `message/engine.go`
- `message/engine_test.go`
- `message/engine_bench_test.go`

**Acceptance Criteria:**
- [ ] Files removed
- [ ] `message` package compiles with no remaining reference to `Engine`, `EngineConfig`, or `Plugin`

### Task 2: Delete Engine-dependent CloudEvents plugin wiring

**Files to delete:**
- `message/cloudevents/plugin.go` (`SubscriberPlugin`, `PublisherPlugin` — pure `Engine`-wiring sugar around `e.AddRawInput`/`e.AddRawOutput`)
- `message/cloudevents/plugin_test.go`

**Kept, unaffected:** `message/cloudevents/subscriber.go`, `publisher.go`, `adapter.go` — these produce/consume raw channels directly and were never `Engine`-dependent. (They still need their own separate `*RawMessage`→`*Message` mechanical port under `marshaling-strategy.md`, unrelated to this task.)

**Acceptance Criteria:**
- [ ] Files removed
- [ ] `message/cloudevents` compiles with no reference to `message.Engine` or `message.Plugin`
- [ ] `message/cloudevents/doc.go` updated to drop plugin-based usage examples

### Task 3: Rewrite tests that wire through Engine

**File:** `message/middleware/correlation_test.go`

Tests `CorrelationID()` via four separate `message.NewEngine(...)` constructions. Rewrite to construct a `Router` directly and drive it through `Router.Pipe()` — the middleware under test doesn't need `Engine`'s orchestration at all.

**Acceptance Criteria:**
- [ ] All `CorrelationID()` test cases pass using `Router` directly
- [ ] No `message.NewEngine` reference remains in the file

### Task 4: Rewrite package docs

**Files:**
- `message/doc.go` — "Quick Start" example is `Engine`-based; rewrite to a `Router`-based quick start (handler registration + `Router.Pipe()` over a channel).
- `message/README.md` — has a full "Engine Architecture" section (diagram + three `Engine` code examples: raw I/O, typed I/O, dynamic input/output). Needs a real rewrite, not a trim: describe `Router` (+ `Merger`/`Distributor` as optional standalone fan-in/fan-out) as the supported composition pattern.

**Acceptance Criteria:**
- [ ] No `message.NewEngine`/`Engine` reference remains in either file
- [ ] Examples in both files compile (verified by doc-testable examples or manual check)

### Task 5: Rewrite example programs

**Files:**
- `examples/04-message/main.go`
- `examples/06-http-cloudevents/main.go`

Both currently demonstrate `Engine` end-to-end. Rewrite to demonstrate `Router` directly (composed with `message/cloudevents` `Subscriber`/`Publisher` for the HTTP example), or drop if no longer representative of a supported pattern.

**Acceptance Criteria:**
- [ ] Both examples run (`go run ./04-message/`, `go run ./06-http-cloudevents/`)
- [ ] Neither references `message.Engine`

### Task 6: ADR and CHANGELOG

- [ ] Write ADR 0028 (next available number) — "Remove Message Engine", superseding ADR 0020, with an `## Updates` note added to ADR 0022 recording that the `Engine` it introduced has since been removed
- [ ] CHANGELOG entry under `[Unreleased]` — Removed: `message.Engine`, `message.EngineConfig`, `message.Plugin`, `message/cloudevents.SubscriberPlugin`/`PublisherPlugin` (breaking, pre-v1)

## Implementation Order

```
Task 1 (delete engine.go + tests)
  → Task 2 (delete cloudevents/plugin.go, depends on Task 1's Plugin type being gone)
  → Task 3 (rewrite correlation_test.go)
  → Task 4 (rewrite doc.go, README.md)
  → Task 5 (rewrite examples)
  → Task 6 (ADR + CHANGELOG, last — documents the completed change)
```

This whole plan must complete before `marshaling-strategy.md`'s Phase 1 (dropping `TypedMessage[T]`/`RawMessage`) starts.

## Acceptance Criteria

- [ ] `make test && make build && make vet` pass with zero references to `Engine`/`EngineConfig`/`Plugin` anywhere in the repository
- [ ] `Router`, `Merger`, `Distributor` confirmed still independently usable and tested without `Engine`
- [ ] ADR 0020 marked Superseded; ADR 0022 has an `## Updates` note
- [ ] CHANGELOG updated
- [ ] This plan's status updated to Complete
