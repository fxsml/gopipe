# PRO-0021: GroupBy Composite Keys

**Status:** Proposed
**Priority:** Low
**Related ADRs:** PRO-0042

## Overview

Add support for composite keys in GroupBy operations.

## Goals

1. Implement `GroupByFunc` with custom equality
2. Create `CompositeKey` helper
3. Add attribute-based key functions

## Task

**Goal:** Support non-comparable keys in grouping

**Files to Create/Modify:**
- `channel/groupby_func.go` (new) - GroupByFunc
- `channel/composite_key.go` (new) - CompositeKey helper
- `channel/key_funcs.go` (new) - Helper functions

**Implementation:**
```go
func GroupByFunc[V any, K any](
    in <-chan V,
    keyFunc func(V) K,
    hasher func(K) uint64,
    equals func(K, K) bool,
    config GroupByConfig,
) <-chan Group[K, V]

type CompositeKey struct { Parts []string }

func GroupByAttributes(attrs ...string) func(*Message) CompositeKey
func StringKeyFunc(attrs ...string) func(*Message) string
```

**Acceptance Criteria:**
- [ ] `GroupByFunc` implemented
- [ ] `CompositeKey` with Hash/Equals
- [ ] `GroupByAttributes` helper
- [ ] `StringKeyFunc` helper
- [ ] Tests for composite grouping
- [ ] CHANGELOG updated

## Related

- [PRO-0042](../../adr/PRO-0042-groupby-composite-keys.md) - ADR
