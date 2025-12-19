# ADR 0042: GroupBy Composite Keys

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #6)

## Context

`GroupBy` requires `comparable` keys, excluding slices/maps. Need support for composite keys from multiple attributes.

## Decision

Add `GroupByFunc` variant and `CompositeKey` helper:

```go
// GroupByFunc with custom equality
func GroupByFunc[V any, K any](
    in <-chan V,
    keyFunc func(V) K,
    hasher func(K) uint64,
    equals func(K, K) bool,
    config GroupByConfig,
) <-chan Group[K, V]

// CompositeKey for multi-attribute grouping
type CompositeKey struct {
    Parts []string
}

func NewCompositeKey(parts ...string) CompositeKey
func (k CompositeKey) String() string
func (k CompositeKey) Hash() uint64
func (k CompositeKey) Equals(other CompositeKey) bool

// Helper for message grouping
func GroupByAttributes(attrs ...string) func(*message.Message) CompositeKey

// Simple string key helper
func StringKeyFunc(attrs ...string) func(*message.Message) string
```

**Usage:**

```go
// Group by tenant + region using CompositeKey
groups := channel.GroupByFunc(
    msgs,
    channel.GroupByAttributes("tenant", "region"),
    func(k CompositeKey) uint64 { return k.Hash() },
    func(a, b CompositeKey) bool { return a.Equals(b) },
    config,
)

// Or simple string keys with existing GroupBy
groups := channel.GroupBy(
    msgs,
    channel.StringKeyFunc("tenant", "region"),
    config,
)
```

## Consequences

**Positive:**
- Support for complex grouping keys
- Backward compatible (new functions)
- Simple string alternative

**Negative:**
- Two similar functions to choose from
- Custom hasher/equals complexity

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)
