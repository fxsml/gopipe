# ADR 0030: Drop Merger, Distributor, and Matcher

**Date:** 2026-07-26
**Status:** Proposed

## Context

`message.Merger`, `message.Distributor`, `pipe.Merger`, `pipe.Distributor`, and the `Matcher` interface (`message/match`) were built as `message.Engine`'s internal fan-in/fan-out plumbing (ADR 0020/0022) and never independently adopted. Checked against real-world evidence — gopipe's own code, a production reference repository, and the `gopipe-azservicebus` broker adapter: zero usage of any of them outside `Engine` itself. `pipe.Distributor`'s only consumer is `message.Distributor`. `pipe.Merger` appeared to have independent evidence via `examples/03-merger`, but its `AddInput` calls all happen before `Merge()` starts — a static merge, functionally identical to `channel.Merge`, that never exercises dynamic add-after-start, the one capability that actually distinguishes it. `Matcher`'s only surviving consumer was `Distributor.AddOutput(matcher)`. Full evidence: [#153](https://github.com/fxsml/gopipe/issues/153).

## Decision

Remove `message.Merger`, `message.Distributor`, `pipe.Merger`, `pipe.Distributor`, the `Matcher` interface, and the `message/match` package (`Types`, `All`, `Any`, `Sources`, `Like`).

`channel.Merge` and `channel.Switch` (renamed from `Route`, [#165](https://github.com/fxsml/gopipe/issues/165)) already cover every real, evidenced fan-in/fan-out need found in this audit. `examples/03-merger` is rewritten to demonstrate `channel.Merge` directly.

All of this is recoverable from git history if a real, evidenced need for dynamic fan-in or matcher-based fan-out ever appears — a reversible cut, not a one-way door.

## Consequences

**Breaking Changes:**
- Removes 4 types, 1 interface, 1 subpackage (pre-v1, acceptable)
- `examples/03-merger` rewritten, no longer demonstrates `pipe.Merger`

**Benefits:**
- Removes unproven API surface, consistent with ADR 0028 rule 4 ("proven, not just plausible, real-world use")
- `channel.Merge`/`Switch` already serve the evidenced need with less API surface

**Drawbacks:**
- Dynamic runtime fan-in (add an input after the merge has started) and matcher-based fan-out have no replacement if a real need for either ever appears — would need reimplementing from git history

## Links

- Decided in [#153](https://github.com/fxsml/gopipe/issues/153) — full evidence and reasoning
- Related: [#152](https://github.com/fxsml/gopipe/issues/152) (`Router` matcher param — same origin pattern), [#165](https://github.com/fxsml/gopipe/issues/165) (`Switch` rename — fan-out vocabulary now two-way)
- Related: ADR 0028 (rule 4, the standard applied here), ADR 0029 (`channel`/`pipe` interface boundaries — a related but separate decision)
