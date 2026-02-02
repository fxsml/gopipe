# ADR 0026: Naming Conventions

**Date:** 2026-02-02
**Status:** Proposed

## Context

The pipe and channel packages need consistent naming for transformer types and operations. Current naming is inconsistent:
- `Transform` vs `Process` for similar operations
- `Generator` doesn't pair well with `Sink`
- No clear convention for 1:1 vs 1:N transformers

## Decision

### 1. Use `-er` Suffix for Transformer Types

Following Go idioms like `Reader`, `Writer`, `Handler`:

| Type | Suffix | Example |
|------|--------|---------|
| 1:1 transformer | `-er` | `Mapper` |
| 1:N transformer | `-er` | `Expander` |
| Predicate | none | `Filter` |

### 2. Source/Sink Pairing

Use `Source` instead of `Generator` to pair semantically with `Sink`:

```go
// Produces items
type Source[Out any] interface {
    Source(ctx context.Context) ([]Out, error)
}

// Consumes items
type Sink[In any] interface {
    Sink(ctx context.Context, in In) error
}
```

### 3. Operation Names

| Operation | Pipe | Channel | Cardinality |
|-----------|------|---------|-------------|
| Transform 1:1 | `Mapper.Map()` | `Map()` | 1:1 |
| Transform 1:N | `Expander.Expand()` | `Expand()` | 1:N |
| Filter | `Filter.Filter()` | `Filter()` | 1:0/1 |
| Consume | `Sink.Sink()` | `Drain()` | terminal |
| Produce | `Source.Source()` | `From*()` | initial |

### 4. Channel-Specific Names

| Function | Meaning |
|----------|---------|
| `Drain` | Consume with handler (side effect) |
| `Drop` | Discard all items (no handler) |
| `Switch` | Fan-out by index (like switch statement) |

## Consequences

**Breaking Changes:**
- `Transform` → `Map` (channel package)
- `Process` → `Expand` (channel package)
- `NewGenerator` → `NewSource` (pipe package)
- `Route` → `Switch` (channel package)

**Benefits:**
- Consistent vocabulary across packages
- Self-documenting operation names
- Follows established Go conventions

**Drawbacks:**
- Breaking changes for existing users
- `Expander` is novel (not widely used in Go ecosystem)

## Links

- Related: [Plan 0013 - Channel Package Refactoring](../plans/0013-channel-package-refactoring.md)
- Related: [Plan 0014 - Pipe Interface Refactoring](../plans/0014-pipe-interface-refactoring.md)
