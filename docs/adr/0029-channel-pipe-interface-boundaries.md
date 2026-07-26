# ADR 0029: Channel/Pipe Interface and Naming Boundaries

**Date:** 2026-07-26
**Status:** Proposed

## Context

Issue [#120](https://github.com/fxsml/gopipe/issues/120) proposed a broad redesign of `channel`, `pipe`, and `middleware` on a branch that never merged (documentation-only, stale, since deleted). As part of v1 preparation, alongside the [channel package usage audit](https://github.com/fxsml/gopipe/issues/164), that proposal was re-evaluated against the current codebase and documented Go interface guidance. Full rationale, sources, and preserved-but-unadopted ideas: [Channel/Pipe Interface Consolidation plan](../plans/channel-pipe-interface-consolidation.md).

## Decision

1. **Keep `channel.Transform`/`Process`/`Sink`/`Drain` as-is.** They already mirror `pipe.NewTransformPipe`/`NewProcessPipe`/`NewSinkPipe` — renaming only `channel`'s side would break that symmetry.
2. **No formal semantic interfaces** (`Filter`/`Mapper`/`Expander`/`Source`/`Processor`/`Sink`) **and no `*Pipe`-suffix rename.** Would define interfaces beside their sole implementation, with no consumer needing them — contrary to documented Go convention.
3. **No `Producer`/`Trigger` abstraction.** No proven real-world need.
4. **`channel.Route` → `channel.Switch`.** Resolves a real naming collision with `message.Router`. Tracked in #165.
5. **`channel.GroupBy` relocates to `pipe`** as `NewGroupByPipe`/`GroupByConfig`, matching the existing `NewBatchPipe`/`BatchConfig` shape. Tracked in #162.
6. **Keep the middleware type-alias fix** (a real Go generics assignability constraint), narrowly scoped. Full `middleware`-module consolidation is deferred pending a `message/middleware` rule-4 evaluation that isn't tracked yet.

## Consequences

**Breaking Changes:**
- `channel.Route` renamed to `channel.Switch` (#165)
- `channel.GroupBy` moves to `pipe` (#162)
- No changes to `Transform`, `Process`, `Sink`, `Drain`, or any `pipe` constructor names

**Benefits:**
- `channel`/`pipe` naming symmetry stays intact
- Avoids interface and abstraction surface with no proven consumer
- Resolves the `Route`/`Router` naming collision

**Drawbacks:**
- Discards several months of exploratory design writing from the retired branch; superseded content is preserved in the consolidation plan and `AGENTS.md`, not repeated here

## Links

- [Channel/Pipe Interface Consolidation plan](../plans/channel-pipe-interface-consolidation.md) — full rationale, sources, deferred ideas
- Related: [#120](https://github.com/fxsml/gopipe/issues/120), [#164](https://github.com/fxsml/gopipe/issues/164), [#160](https://github.com/fxsml/gopipe/issues/160)–[#163](https://github.com/fxsml/gopipe/issues/163), [#165](https://github.com/fxsml/gopipe/issues/165)
- Related: ADR 0028 (External Dependency Policy) — same "proven, not just plausible" standard applied here to interface surface
