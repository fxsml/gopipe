# ADR 0015: Middleware Pattern

**Date:** 2024-10-01
**Status:** Implemented

## Context

With the Processor abstraction, gopipe needs a standardized way to extend processor functionality with cross-cutting concerns (logging, metrics, error handling).

## Decision

Introduce middleware pattern:

```go
type MiddlewareFunc[In, Out any] func(Processor[In, Out]) Processor[In, Out]
```

Middlewares are applied in reverse order (last to first), creating a wrapping pattern. Core concerns exposed via dedicated options with enforced ordering:
1. Cancel override (innermost)
2. Context (timeout/propagation)
3. Metrics (includes logger)
4. User custom middleware
5. Metadata providers
6. Recover (outermost)

## Consequences

**Positive:**
- Clean separation of cross-cutting concerns
- Consistent extension pattern
- Reusable functionality across processors

**Negative:**
- Complexity in understanding execution flow
- Slight performance overhead

## Links

- Related: ADR 0013, ADR 0014
