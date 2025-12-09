# ADR 0013: Processor Abstraction

**Date:** 2024-10-01
**Status:** Implemented

## Context

The gopipe library needs a consistent way to handle processing with support for cancellation, ensuring clean control flow and resource management.

## Decision

Introduce a Processor interface that combines processing and error handling as a single unit:

```go
type Processor[In, Out any] interface {
    Process(context.Context, In) (Out, error)
    Cancel(In, error)
}
```

## Consequences

**Positive:**
- Unified abstraction for processing and error handling
- Enables middleware pattern for cross-cutting concerns
- Improves composability and testability

**Negative:**
- Adds abstraction layer overhead
- Requires refactoring existing code

## Links

- Related: ADR 0014, ADR 0015
