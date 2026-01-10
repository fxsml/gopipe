# ADR 0001: Processor Abstraction

**Date:** 2025-10-01
**Status:** Superseded by ADR 0015

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

**Benefits:**
- Unified abstraction for processing and error handling
- Enables middleware pattern for cross-cutting concerns
- Improves composability and testability

**Drawbacks:**
- Adds abstraction layer overhead
- Requires refactoring existing code

## Links

- Superseded by: ADR 0015 (Remove Cancel Path)

## Updates

**2025-12-22:** Updated Consequences format to match ADR template.
