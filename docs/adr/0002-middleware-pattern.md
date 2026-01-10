# ADR 0002: Middleware Pattern

**Date:** 2025-10-01
**Status:** Superseded by ADR 0017

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

**Benefits:**
- Clean separation of cross-cutting concerns
- Consistent extension pattern
- Reusable functionality across processors

**Drawbacks:**
- Complexity in understanding execution flow
- Slight performance overhead

## Links

- Superseded by: ADR 0017 (Middleware for ProcessFunc)

## Updates

**2025-12-22:** Updated Consequences format to match ADR template.
