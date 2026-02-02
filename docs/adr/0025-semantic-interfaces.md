# ADR 0025: Semantic Interfaces Pattern

**Date:** 2026-02-02
**Status:** Proposed

## Context

The pipe package uses `Pipe[In, Out]` as the universal composition interface. However, this single interface obscures what each component actually does. Users cannot tell from the type signature whether a `Pipe[In, Out]` is a transformer, filter, source, or sink.

Additionally, there's no way to invoke the core operation directly without going through the pipe machinery (middleware, error handling, etc.).

## Decision

Each component type exposes both:
1. A **semantic interface** with a direct method reflecting its purpose
2. A **composition interface** (`Pipe()`) for pipeline construction

### Pattern

```go
// Semantic interface - exposes direct operation
type Mapper[In, Out any] interface {
    Map(in In) Out
}

// Composition struct - implements both semantic and Pipe interfaces
type MapperPipe[In, Out any] struct {
    // internal fields
}

// Direct method - no middleware, no error handling
func (m *MapperPipe[In, Out]) Map(in In) Out {
    return m.fn(in)
}

// Composition method - returns Pipe for pipeline use
func (m *MapperPipe[In, Out]) Pipe() Pipe[In, Out] {
    return m.pipe
}
```

### Struct Naming Convention

All composition structs use `*Pipe` suffix:
- `SourcePipe[Out]`
- `ProcessorPipe[In, Out]`
- `MapperPipe[In, Out]`
- `ExpanderPipe[In, Out]`
- `FilterPipe[T]`
- `SinkPipe[In]`

### Usage

```go
// Direct invocation (testing, simple cases)
mapper := pipe.NewMapper(strings.ToUpper)
result := mapper.Map("hello")  // "HELLO"

// Pipeline composition
pipeline := pipe.Join(
    source.Pipe(),
    mapper.Pipe(),
    sink.Pipe(),
)
```

## Consequences

**Breaking Changes:**
- Constructor renames: `NewProcessPipe` → `NewProcessor`, etc.
- Return types change to semantic structs

**Benefits:**
- Self-documenting types: `MapperPipe` vs opaque `Pipe`
- Direct access to core operation for testing
- Clear separation of concerns

**Drawbacks:**
- More types to understand
- Two ways to invoke (direct vs pipe)

## Links

- Related: [Plan 0014 - Pipe Interface Refactoring](../plans/0014-pipe-interface-refactoring.md)
- Related: ADR 0024 (Pure vs Impure Operations)
