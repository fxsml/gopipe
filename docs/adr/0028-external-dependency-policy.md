# ADR 0028: External Dependency Policy

**Date:** 2026-07-26
**Status:** Proposed

## Context

gopipe is a multi-module repository: `channel`, `pipe`, `message`, `message`'s subpackages (`http`, `cloudevents`, `jsonschema`, `match`, `middleware`), and the separately-versioned `message/otel`. Reviewing the marshaling-strategy work ([#125](https://github.com/fxsml/gopipe/issues/125), [#147](https://github.com/fxsml/gopipe/issues/147)–[#149](https://github.com/fxsml/gopipe/issues/149)) and its knock-on findings ([#151](https://github.com/fxsml/gopipe/issues/151)–[#154](https://github.com/fxsml/gopipe/issues/154)) surfaced recurring, previously undocumented questions about which modules may depend on external libraries, and which external-ecosystem bridges belong in this repository at all. Without an explicit policy these decisions were made ad hoc: core `message`'s `go.mod` pulled in `cloudevents/sdk-go` and `santhosh-tekuri/jsonschema` for every consumer regardless of need, and `message/otel` was split into a same-repo module (unreleased, unproven) rather than following the separate-repo convention already established by `gopipe-azservicebus`.

## Decision

Four rules govern dependencies and package placement across the repository:

1. **`channel` and `pipe` allow zero external dependencies — including test-only dependencies.** Standard library only, no exceptions. These are the foundational modules; every other module depends on them, so anything pulled in here propagates everywhere. (Verified currently true: `channel/go.mod` has no `require` block at all; `pipe/go.mod` requires only `channel`.)

2. **`message` (core) allows external dependencies, but only curated and individually justified ones** — each weighed for real benefit against its cost (transitive footprint, maintenance burden, fit with gopipe's own stdlib-first conventions). `google/uuid` is the reference example: small, stable, clear benefit, no controversy.

3. **No gopipe package's public API may ever expose a type from an external library.** If a package's contract requires a consumer to also import a third-party SDK directly (e.g. `cloudevents.Event`, `protocol.Receiver`/`Sender`, an OTel `metric.Meter`), that package does not live in this repository — it lives in its own separate repository, following the naming convention already established by `gopipe-azservicebus` (e.g. `gopipe-cloudevents`, `gopipe-otel`).

4. **A subpackage must be stable, mature, and have a demonstrated real-world use case to ship — independent of whether it passes rules 1–3.** Passing the dependency rules is necessary but not sufficient; an immature or effectively-unused abstraction doesn't ship in v1 merely because it's dependency-clean. (`message/match`/`Matcher`'s real-world usage is evaluated under this rule via [#153](https://github.com/fxsml/gopipe/issues/153); `message/middleware`'s five middlewares are evaluated individually, not collectively, since passing rules 1–3 trivially — zero external deps in any of them — doesn't itself satisfy rule 4.)

## Consequences

**Breaking Changes:**

- `message/cloudevents` moves to its own repository (rule 3) — no longer importable as `github.com/fxsml/gopipe/message/cloudevents`.
- `message/otel` moves to its own repository (rule 3), replacing its current same-repo-module shape.
- `message/jsonschema` is removed entirely (rule 4), decided, not tentative — its wrapper `Registry` duplicates `Router`'s own eventType→handler bookkeeping in a second, independently-maintained mapping. The underlying `santhosh-tekuri/jsonschema` library itself is not the problem (it passes rule 2 on its own merits) and remains fine to use directly in application code; gopipe just stops shipping an abstraction around it. `examples/07-validating-marshaler` is rewritten (not deleted) to demonstrate the replacement, also decided: a `Marshaler` decorator keying schemas by `reflect.Type` instead of a separately-derived event-type string, sidestepping the naming-drift failure mode structurally rather than by convention. The only open-ended part is further out: whether shipped JSON Schema support as a real package *ever* returns — worth reconsidering only if a future design earns back the schema-catalog/proxy-validation capabilities traded away here without reintroducing a second registry. That's a distant possibility, not a qualifier on the removal itself.
- `message/http` keeps CloudEvents HTTP protocol binding support, but implements it directly rather than depending on `cloudevents/sdk-go` (rule 2 weighed against it: the SDK's transitive footprint includes a full `zap` logging framework and `json-iterator` alternate JSON encoder, both duplicating capabilities gopipe already has via `message.Logger` and stdlib `encoding/json`). Structured and batch mode reuse existing `message.go` logic (`ParseRaw`/`WriteTo` already implement CloudEvents' structured-mode JSON envelope); binary mode is implemented directly against `*message.RawMessage`.

**Benefits:**

- Consumers of core `github.com/fxsml/gopipe/message` never carry dependencies for features they don't use.
- Clear, checkable rules for evaluating any future subpackage or dependency addition.
- `message/http` becomes genuinely zero-dependency — the "batteries included, works without a broker" reference transport, not "zero dependency except transitively."

**Drawbacks:**

- `message/http` owns correctness for CloudEvents HTTP binary-mode encoding (percent-encoding rules, UTF-8 validation) itself, rather than inheriting the official SDK's cross-implementation-tested behavior.
- More repositories to maintain and release independently (`gopipe-cloudevents`, `gopipe-otel`) instead of one.

## Links

- Related: [#125](https://github.com/fxsml/gopipe/issues/125), [#147](https://github.com/fxsml/gopipe/issues/147), [#148](https://github.com/fxsml/gopipe/issues/148), [#149](https://github.com/fxsml/gopipe/issues/149), [#151](https://github.com/fxsml/gopipe/issues/151), [#152](https://github.com/fxsml/gopipe/issues/152), [#153](https://github.com/fxsml/gopipe/issues/153), [#154](https://github.com/fxsml/gopipe/issues/154)
- Related: ADR 0020 (Message Engine Architecture), ADR 0022 (Message Package Redesign)
- Supersedes: ADR 0027 (JSON Schema Validation) — `message/jsonschema` is removed per rule 4's consequences above
