# ADR 0004: Remove Functional Options from Message Construction

**Date:** 2025-12-07
**Status:** Implemented

## Context

Message API used functional options with generic type parameters (`WithID[T]()`, `WithAcking[T]()`), creating visual clutter. Every option required a type parameter redundant with the message type.

## Decision

Remove functional options and use simple constructors:

```go
// Default constructor (no acknowledgment)
func New[T any](data T, props Attributes) *TypedMessage[T]

// Constructor with acknowledgment callbacks
func NewWithAcking[T any](data T, props Attributes, ack func(), nack func(error)) *TypedMessage[T]

// Attributes type alias
type Attributes map[string]any
```

## Consequences

**Breaking Changes:**
- All existing code using functional options must be updated

**Benefits:**
- No type parameters on options
- Attributes visible at construction time
- Follows standard Go patterns
- Attributes can be built incrementally and reused

**Drawbacks:**
- Less discoverable via IDE autocomplete
- More verbose for single property (need to create map)

## Links

- Related: ADR 0007 (Public Message Fields)
- Related: ADR 0010 (Dual Message Types)

## Updates

**2025-12-22:** Fixed Links references. Updated Consequences format to match ADR template.
