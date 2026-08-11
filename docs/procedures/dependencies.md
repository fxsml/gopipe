# Dependency & Package Boundary Procedures

Actionable checklist for external dependencies and package placement across the gopipe multi-module repository. Full rationale and case studies: [ADR 0028](../adr/0028-external-dependency-policy.md).

## Before Adding an External Dependency

### `channel` and `pipe`

- [ ] Stop. Zero external dependencies allowed — including test-only. No exceptions, regardless of how small or well-regarded the library is. Verify: `cat channel/go.mod pipe/go.mod` should show no `require` beyond `github.com/fxsml/gopipe/channel` (for `pipe`).

### `message` (core)

- [ ] Does this dependency deliver real, individually justified benefit against its transitive cost? (`google/uuid` is the reference example: small, stable, clear win, no controversy.)
- [ ] Check the footprint at the **module** level, not the direct `require` line — a small direct dependency can still pull in a heavy transitive graph:
  ```bash
  cd message && GOWORK=off go list -deps ./... | grep -v "^github.com/fxsml"
  ```
- [ ] Does it duplicate a capability gopipe already has via stdlib or its own abstractions (`message.Logger`, stdlib `encoding/json`)? A dependency that ships its own logging framework or its own JSON codec is a much harder sell than one that doesn't, even at the same raw dependency count — see ADR 0028's `cloudevents/sdk-go` case (`zap`, `json-iterator`) vs. `santhosh-tekuri/jsonschema` (mostly `golang.org/x/text`, already stdlib-adjacent) for the concrete comparison.

### Any package's public API

- [ ] Does any exported function or type take or return a type from an external library? Check directly, don't eyeball it:
  ```bash
  grep -n "^func \|^type " path/to/package/*.go | grep -v "_test.go"
  ```
  Cross-reference the return/parameter types against the package's imports. If an external type appears in an exported signature, **this package does not belong in this repository.** It belongs in its own repository, named `gopipe-<name>`, following the convention already established by `gopipe-azservicebus`.

## Before Shipping a New Subpackage

A subpackage must clear **both** of the following — passing one without the other is not enough:

- [ ] **Dependency-clean**, per the checks above.
- [ ] **Proven, not just plausible, real-world use.** Check actual call sites, not intent or design docs:
  ```bash
  grep -rn "packagename\." --include="*.go" . | grep -v "_test.go"
  ```
  Zero or near-zero real (non-test) call sites is a signal the subpackage is speculative, however clean its dependency story is. This is what caught `Router`'s handler-level `Matcher` parameter (`#152`) — every real call site passed `nil`.
- [ ] The subpackage's own design is mature, independent of whether its underlying dependency is. `message/jsonschema` is the cautionary example: `santhosh-tekuri/jsonschema` itself was a reasonable dependency — the wrapper `Registry` gopipe built around it had a real design flaw (a second, parallel eventType↔type mapping that could silently drift from `Router`'s own registry). The dependency passed; the package didn't.

## Quick Reference

| Module | External deps allowed? | Notes |
|---|---|---|
| `channel` | No | Stdlib only, no exceptions, including tests |
| `pipe` | No | Stdlib only, plus `channel` |
| `message` (core) | Yes, curated | Individually justified per dependency, e.g. `google/uuid` |
| Any subpackage whose public API exposes a foreign type | N/A | Doesn't live in this repo — separate `gopipe-<name>` repository |

## Related

- [ADR 0028](../adr/0028-external-dependency-policy.md) — full decision record, four rules, consequences
- [ADR 0027](../adr/0027-json-schema-validation.md) — superseded by 0028; the `message/jsonschema` parallel-registry case study
