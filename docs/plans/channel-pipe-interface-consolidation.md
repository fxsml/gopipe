# Plan: Channel/Pipe Interface Consolidation

**Status:** In Progress
**Related ADRs:** [0029](../adr/0029-channel-pipe-interface-boundaries.md)

## Overview

Issue [#120](https://github.com/fxsml/gopipe/issues/120) and its branch `claude/refactor-pipe-interfaces-7NuOQ` proposed a broad redesign of `channel`, `pipe`, and `middleware`: formal pure/impure semantic interfaces, a `Producer`/`Trigger` abstraction, and a `channel`-package renaming pass. None of its seven draft ADRs (0024–0030) or four plans (0013–0016) ever merged to `develop` — the branch was documentation-only (zero `.go` changes across 13 files) and stale (diverged 2026-01-28, last commit 2026-02-02, no PR opened). Its ADR numbers (0024–0028) and plan number (0013) have since been reused on `develop` for unrelated work, and the branch itself has been deleted (no GitHub archive mechanism exists short of keeping it around).

This plan re-evaluates that proposal end to end against the current codebase and documented Go interface guidance — in conjunction with the [channel package usage audit](https://github.com/fxsml/gopipe/issues/164) (#160–#163) — and is the durable record of what was proposed, what survived, and why, now that the source branch is gone. [ADR 0029](../adr/0029-channel-pipe-interface-boundaries.md) records only the resulting decisions; this plan carries the reasoning, sources, and everything not adopted.

## Goals

1. Decide, with evidence rather than preference, which parts of the old proposal to adopt, reject, or defer
2. Preserve enough of the retired branch's content that nothing useful is lost on deletion
3. Track the resulting concrete changes to closure

## Decisions & Rationale

### 1. Preserve `channel`/`pipe` verb symmetry — reject the `channel`-only renaming pass

`pipe/pipe.go` already exposes `NewTransformPipe`, `NewProcessPipe`, and `NewSinkPipe` — the same verbs as `channel.Transform`, `channel.Process`, `channel.Sink`: the same word names the same operation at both purity tiers (pure/stateless in `channel`, impure/stateful in `pipe`). The old branch's renames (`Transform`→`Map`, `Process`→`Expand`, `Sink`→`Drain`, `Drain`→`Drop`) only touched `channel`'s side, which would have broken that symmetry; renaming both sides would double the churn for names that are already clear. **Kept as-is.** (`Merge`/`Merger` was originally cited here as another instance of the same pattern; `pipe.Merger` is now dropped per #153, so this argument rests on `Transform`/`Process`/`Sink` alone — still sufficient on its own.)

### 2. Reject formal semantic interfaces and the `*Pipe`-suffix rename

The [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) wiki: *"Go interfaces generally belong in the package that uses values of the interface type, not the package that implements those values... Do not define interfaces before they are used."* It labels the opposite — a producer package defining an interface next to its only implementation — "DO NOT DO IT!!!". The old branch's `Mapper`/`Expander`/`Filter`/`Source`/`Processor`/`Sink` interfaces would have lived in `pipe` beside their sole implementations (`MapperPipe`, etc.), with no consumer needing them for polymorphism. Their stated purpose — direct invocation without pipe machinery, for testing — is also moot: every constructor (e.g. `NewProcessPipe(handleFunc, cfg)`) already takes the raw handler function as a parameter, so callers already hold it. The underlying pure/impure *observation* (`channel` = no ctx/error, `pipe` = ctx/error) is correct and already documented in `channel/doc.go`/`pipe/doc.go` — it doesn't need formal interfaces to be true.

### 3. Reject `Producer`/`Trigger` separation

No proven real-world need. The one production periodic-generation use case identified (in the production reference repository used for v1 evidence-gathering) implements its own scheduling entirely from scratch — distributed-lock renewal, a custom cycle loop, its own config — without using `pipe.Generator` or `channel.FromFunc`. A generic `trigger.Interval`/`trigger.Cron` abstraction would not have simplified that real case; the complexity lived in domain logic, not in "when to trigger."

### 4. Adopt `channel.Route` → `channel.Switch`

Real naming collision: `channel.Route` (a function) and `message.Router` (a type) coexist across the module family and are conceptually unrelated (index-based fan-out vs. event-type routing with handlers). `Switch` reads unambiguously as "select one output by index," matching Go's own `switch` statement. Tracked in #165.

**Updated:** this decision originally framed `Switch` as completing a three-way fan-out vocabulary alongside `Broadcast` and `pipe.Distributor` ("select one by matcher"). `pipe.Distributor` (and `message.Distributor`, `message.Merger`, `pipe.Merger`, and `Matcher`/`message/match`) are now dropped entirely — decided in #153, zero evidence anywhere for matcher-based fan-out or dynamic add-after-start merging once checked properly (see #153 for the full reasoning, including why `pipe.Merger`'s one apparent example didn't actually hold up). The fan-out vocabulary is now two-way: `Broadcast` (copy to all) and `Switch` (select one by index) — both covered by real, evidenced usage.

### 5. `GroupBy` relocates to `pipe`, existing constructor pattern, no new interface

Tracked in #162. Lands as `NewGroupByPipe`/`GroupByConfig`, matching the shape `NewBatchPipe`/`BatchConfig` already uses — a concrete constructor returning a concrete struct, consistent with decision 2 and Code Review Comments' guidance that implementing packages return concrete types.

### 6. Keep the middleware type-alias fix, narrowly scoped — defer full `middleware` module consolidation

The old branch's middleware work bundled two different things: (a) a real Go generics constraint and its fix, and (b) a structural consolidation of `pipe/middleware` and `message/middleware` into one top-level `middleware` module. Only (a) is adopted.

The constraint is real, not a style preference. Raw function types are not assignable to a named generic type in variadic parameters:

```go
// Does NOT work — raw signature, fails at the call site
func Retry[In, Out any]() func(func(context.Context, In) ([]Out, error)) func(context.Context, In) ([]Out, error)
pipe.Use(processor, Retry[string, string]())  // ERROR: type mismatch

// Works — middleware constructors return pipe.ProcessFunc directly
func Retry[In, Out any]() func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]
pipe.Use(processor, Retry[string, string]())  // OK: assignable to pipe.Middleware
```

Rule going forward: any future `middleware` package must define no types of its own and use `pipe.ProcessFunc[In, Out]` directly; a message-aware variant needs only a type *alias* (`type Middleware = pipe.Middleware[*message.Message, *message.Message]`), not a new type, to stay interchangeable with the generic form.

**Not adopted yet:** the full `middleware`-module consolidation (moving `pipe/middleware` and `message/middleware` into one tree, third-party-integration isolation, deprecating the old packages). ADR 0028's rule 4 ("proven, not just plausible, real-world use") applies here too — `message/middleware`'s middlewares are meant to be evaluated individually before any consolidation decision, and that evaluation has no tracking issue yet. Consolidating before knowing which middlewares survive risks packaging together things that don't all belong. Revisit once that evaluation is tracked and run.

## Preserved for Later: Unevaluated Ideas from the Retired Branch

Outside this plan's scope, never evaluated against real usage. Recorded so they aren't lost, not acted on:

- **Router `PreMap`/`PostMap`** — optional type-conversion hooks on `RouterConfig` so handlers can work with domain-specific types while `Router` converts at the boundary. The original proposal itself left this undecided ("middleware may be sufficient for enrichment/normalization use cases").
- **Conditional/branching pipes** (`pipe.NewBranch(predicate, truePipe, falsePipe)`) — route to one of two pipes by predicate. No evidence of need surfaced during this review.
- **Error-recovery pipes** (`pipe.NewRecover(mainPipe, fallbackFunc)`) — on error, call a fallback instead of failing the pipeline. Distinct from `middleware.Recover` (panic recovery, not error recovery). No evidence of need surfaced during this review.

One related idea from the same source was already rejected by its own authors, and that reasoning holds (recorded in `AGENTS.md` Rejected Alternatives): giving every pipe type a `Process()` method via composition (so any pipe satisfies a `Processor` interface) was rejected for wrapping pure single-value results in a slice unnecessarily and blurring the pure/impure distinction decision 2 depends on.

## Tasks

- [ ] #160 — Remove `channel.FromRange`
- [ ] #161 — Remove `channel.Cancel`
- [ ] #162 — Relocate `channel.GroupBy` to `pipe`
- [ ] #163 — Fix `channel.ToSlice`
- [ ] #165 — Rename `channel.Route` to `channel.Switch`
- [ ] #153 — Drop `message.Merger`/`Distributor`, `pipe.Merger`/`Distributor`, `Matcher`/`message/match`
- [ ] Close #120, superseded by this plan and #164
- [ ] File a tracking issue for the `message/middleware` rule-4 evaluation gating decision 6's consolidation question (not yet filed)

## Acceptance Criteria

- [ ] All linked issues closed
- [ ] `docs/adr/0029-channel-pipe-interface-boundaries.md` reflects final state
- [ ] `make test && make build && make vet` pass

## Sources

- [Go Code Review Comments — Interfaces](https://go.dev/wiki/CodeReviewComments)
- [Effective Go — Interface names](https://go.dev/doc/effective_go)
- Production reference repository (v1 evidence-gathering) and `gopipe-azservicebus` — see #164
