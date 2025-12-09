# ADR 0005: Remove Functional Options from Message Construction

**Date:** 2024-12-07
**Status:** Implemented

## Context

Message API used functional options with generic type parameters (`WithID[T]()`, `WithAcking[T]()`), creating visual clutter. Every option required a type parameter redundant with the message type.

## Decision

Remove functional options and use simple constructors:

```go
// Default constructor (no acknowledgment)
func New[T any](payload T, props Properties) *TypedMessage[T]

// Constructor with acknowledgment callbacks
func NewWithAcking[T any](payload T, props Properties, ack func(), nack func(error)) *TypedMessage[T]

// Properties type alias
type Properties map[string]any
```

## Consequences

**Positive:**
- No type parameters on options
- Properties visible at construction time
- Follows standard Go patterns
- Properties can be built incrementally and reused

**Negative:**
- Breaking change: all existing code must be updated
- Less discoverable via IDE autocomplete
- More verbose for single property (need to create map)

## Links

- ADR 0001: Public Message Fields
- ADR 0004: Dual Message Types
- Supersedes: ADR-3 Functional Options
