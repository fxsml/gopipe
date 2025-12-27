# ADR 0018: Interface Naming Conventions

**Date:** 2025-12-22
**Status:** Implemented

## Context

Inconsistent interface naming across library. Go convention: `<Verb>er` interface with `<Verb>()` method (e.g., `io.Reader.Read()`).

## Decision

Adopt `<Verb>er.<Verb>()` pattern. Reserve `Start()` for orchestration types only.

```go
// Channel-level operations
type Pipe[In, Out any] interface {
    Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}

type Generator[Out any] interface {
    Generate(ctx context.Context) (<-chan Out, error)
}

type Merger[T any] interface {
    AddInput(ch <-chan T) (<-chan struct{}, error)
    Merge(ctx context.Context) (<-chan T, error)
}

type Distributor[T any] interface {
    AddOutput(matcher func(T) bool) (<-chan T, error)
    Distribute(ctx context.Context, in <-chan T) (<-chan struct{}, error)
}

// Orchestration (Start reserved)
type Engine interface {
    Start(ctx context.Context) (<-chan struct{}, error)
}
```

## Consequences

**Breaking Changes:**
- `Pipe.Start()` → `Pipe.Pipe()`
- `FanIn` → `Merger`, `FanInConfig` → `MergerConfig`
- `FanIn.Start()` → `Merger.Merge()`
- `Merger.Add()` → `Merger.AddInput()` (added 2025-12-28 for symmetry with `Distributor.AddOutput()`)

**Benefits:**
- Consistent `<Verb>er.<Verb>()` pattern across library
- `Start()` clearly indicates orchestration-level component
- Symmetric `AddInput`/`AddOutput` naming for inverse operations (Merger/Distributor)

**Drawbacks:**
- Breaking change for existing `Pipe` and `FanIn` users

## Links

- Related: ADR 0015-0017 (processor simplification series)
