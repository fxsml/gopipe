# PRO-0007: GroupBy Key Flexibility

**Status:** Proposed
**Priority:** Low

## Overview

Improve `channel.GroupBy` to support non-comparable keys via custom hash/equals functions.

## Problem

`channel.GroupBy` requires `comparable` keys:

```go
func GroupBy[V any, K comparable](
    in <-chan V,
    keyFunc func(V) K,
    config GroupByConfig,
) <-chan Group[K, V]
```

This excludes:
- Slice keys (grouping by multiple attributes)
- Map keys
- Struct keys with slice/map fields

## Current Workaround

Convert complex keys to strings:

```go
keyFunc := func(msg *Message) string {
    return fmt.Sprintf("%s:%s", msg.Attributes["tenant"], msg.Attributes["region"])
}
```

## Proposed Solution

Add `GroupByFunc` with custom hasher:

```go
type KeyHasher[K any] interface {
    Hash(K) uint64
    Equal(K, K) bool
}

func GroupByFunc[V any, K any](
    in <-chan V,
    keyFunc func(V) K,
    hasher KeyHasher[K],
    config GroupByConfig,
) <-chan Group[K, V]

// Helper for common composite keys
type CompositeKey []string

func NewCompositeKeyHasher() KeyHasher[CompositeKey]
```

## Task

**Files to Create/Modify:**
- `channel/groupby.go` (modify) - Add GroupByFunc
- `channel/hasher.go` (new) - KeyHasher interface and helpers

**Acceptance Criteria:**
- [ ] GroupByFunc with custom hasher works
- [ ] CompositeKey helper provided
- [ ] Original GroupBy unchanged
- [ ] Tests pass
- [ ] CHANGELOG updated

## Related

- [channel/groupby.go](../../../channel/groupby.go) - Current implementation
