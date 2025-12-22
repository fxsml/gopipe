# ADR 0010: Dual Message Types

**Date:** 2025-12-07
**Status:** Implemented

## Context

gopipe serves two use cases: messaging/pub-sub (datas are `[]byte`) and type-safe pipelines (compile-time safety). Using generic `Message[T]` for both created friction for pub/sub patterns with verbose type parameters.

## Decision

Introduce a dual-type system using a type alias:

```go
// Generic typed message for type-safe pipelines
type TypedMessage[T any] struct {
    Data    T
    Attributes map[string]any
}

// Non-generic message alias for pub/sub
type Message = TypedMessage[[]byte]
```

## Consequences

**Benefits:**
- Clean API for pub/sub (no `[[]byte]` type parameters)
- Type-safe pipelines preserved with `TypedMessage[T]`
- Zero runtime overhead (type alias is compile-time only)
- Code reuse: all methods work for both types

**Drawbacks:**
- Users must choose appropriate type for their use case
- `TypedMessage` naming less intuitive than `Message`

## Links

- Related: ADR 0007 (Public Message Fields)
- Related: ADR 0008 (Remove Attributes Thread-Safety)

## Updates

**2025-12-22:** Fixed Links references. Updated Consequences format to match ADR template.
