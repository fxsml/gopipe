# ADR 0024: Pure vs Impure Operations

**Date:** 2026-02-02
**Status:** Proposed

## Context

The pipe package needs to distinguish between operations that are deterministic transformations and operations that may involve I/O, side effects, or failure. Currently, all operations use the same signature with `context.Context` and `error` return, even when they are pure transformations.

This creates unnecessary complexity for simple transformations like filtering or mapping, where:
- Context is unused (no cancellation, no deadlines)
- Errors cannot occur (deterministic transformation)
- The operation is always successful

## Decision

Separate operations into **pure** and **impure** categories based on their signatures:

### Pure Operations (no ctx, no error)

```go
// Filter - predicate returns bool
type Filter[T any] interface {
    Filter(in T) bool
}

// Mapper - 1:1 transformation
type Mapper[In, Out any] interface {
    Map(in In) Out
}

// Expander - 1:N transformation
type Expander[In, Out any] interface {
    Expand(in In) []Out
}
```

### Impure Operations (with ctx and error)

```go
// Source - produces items, may involve I/O
type Source[Out any] interface {
    Source(ctx context.Context) ([]Out, error)
}

// Processor - 1:N with potential failure
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

// BatchProcessor - batch processing with potential failure
type BatchProcessor[In, Out any] interface {
    ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
}

// Sink - consumes items, may involve I/O
type Sink[In any] interface {
    Sink(ctx context.Context, in In) error
}
```

## Consequences

**Breaking Changes:**
- Existing `TransformFunc` users must migrate to `Mapper` (simpler signature)
- Existing `FilterFunc` users benefit from simpler signature (no error return)

**Benefits:**
- Clearer intent: pure operations cannot fail
- Simpler signatures for common transformations
- Better type safety: compiler enforces purity
- Easier testing: pure operations are deterministic

**Drawbacks:**
- Two categories to understand
- Migration effort for existing code

## Links

- Related: [Plan 0014 - Pipe Interface Refactoring](../plans/0014-pipe-interface-refactoring.md)
- Related: ADR 0025 (Semantic Interfaces Pattern)
