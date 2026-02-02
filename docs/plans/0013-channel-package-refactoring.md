# Plan 0013: Channel Package Refactoring

**Status:** Proposed
**Related:** [0014 - Pipe Interface Refactoring](./0014-pipe-interface-refactoring.md)

## Overview

Refactor the channel package for naming consistency with the pipe package refactoring (Plan 0014). The channel package provides **pure, stateless** channel operations, while pipe provides **stateful** components with lifecycle management.

## Goals

1. Consistent naming between channel and pipe packages
2. Clear semantic distinction between operations
3. Align with Go `-er` convention where applicable

## Naming Alignment

| Concept | Channel (Before) | Channel (After) | Pipe (Plan 0014) |
|---------|------------------|-----------------|------------------|
| Pure 1:1 | `Transform` | `Map` | `Mapper.Map()` |
| Pure 1:N | `Process` | `Expand` | `Expander.Expand()` |
| Pure Filter | `Filter` | `Filter` (unchanged) | `Filter.Filter()` |
| Consume with side effect | `Sink` | `Drain` | `Sink.Sink()` (impure) |
| Discard all | `Drain` | `Drop` | N/A |

## Breaking Changes

### 1. Rename `Transform` to `Map`

**File:** `channel/transform.go` → `channel/map.go`

```go
// Before
func Transform[In, Out any](in <-chan In, handle func(In) Out) <-chan Out

// After
func Map[In, Out any](in <-chan In, handle func(In) Out) <-chan Out
```

### 2. Rename `Process` to `Expand`

**File:** `channel/process.go` → `channel/expand.go`

```go
// Before
func Process[In, Out any](in <-chan In, handle func(In) []Out) <-chan Out

// After
func Expand[In, Out any](in <-chan In, handle func(In) []Out) <-chan Out
```

### 3. Rename `Sink` to `Drain`

**File:** `channel/sink.go` → `channel/drain.go` (merge with old drain.go)

```go
// Before
func Sink[T any](in <-chan T, handle func(T)) <-chan struct{}

// After
func Drain[T any](in <-chan T, handle func(T)) <-chan struct{}
```

### 4. Rename `Drain` to `Drop`

**File:** `channel/drain.go` (old behavior moves to Drop)

```go
// Before
func Drain[T any](in <-chan T) <-chan struct{}

// After
func Drop[T any](in <-chan T) <-chan struct{}
```

## No Changes Required

| Function | Reason |
|----------|--------|
| `Filter` | Already matches pipe naming |
| `Flatten` | Different operation (unbatch), no pipe equivalent |
| `Buffer` | Utility, no naming conflict |
| `FromSlice`, `FromValues`, `FromRange`, `FromFunc` | Source utilities |
| `ToSlice` | Sink utility |
| `Collect`, `Batch`, `GroupBy` | Batching operations |
| `Merge`, `Broadcast`, `Route` | Fan-in/out operations |
| `Cancel` | Control flow |

## Tasks

### Task 1: Rename Transform to Map

**Files to modify:**
- `channel/transform.go` → rename to `channel/map.go`
- `channel/transform_test.go` → rename to `channel/map_test.go`
- `channel/internal/test/transform.go` → rename to `channel/internal/test/map.go`

**Changes:**
- Rename function `Transform` → `Map`
- Update all internal references

### Task 2: Rename Process to Expand

**Files to modify:**
- `channel/process.go` → rename to `channel/expand.go`
- `channel/process_test.go` → rename to `channel/expand_test.go`
- `channel/internal/test/process.go` → rename to `channel/internal/test/expand.go`

**Changes:**
- Rename function `Process` → `Expand`
- Update all internal references

### Task 3: Rename Sink to Drain, Drain to Drop

**Files to modify:**
- `channel/sink.go` → rename to `channel/drain.go`
- `channel/drain.go` → content moves to new function `Drop` in same file
- `channel/sink_test.go` → rename to `channel/drain_test.go`
- `channel/drain_test.go` → merge into `channel/drain_test.go` or create `channel/drop_test.go`
- `channel/internal/test/sink.go` → rename to `channel/internal/test/drain.go`

**Changes:**
- Rename function `Sink` → `Drain`
- Rename function `Drain` → `Drop`
- Consolidate into `channel/drain.go` with both `Drain` and `Drop`

### Task 4: Update Documentation

**Files to modify:**
- `channel/doc.go` - Update package documentation and examples
- `README.md` - Update examples
- `AGENTS.md` - Update design guidance

### Task 5: Update Examples

**Files to modify:**
- `examples/01-channel/main.go`
- `examples/02-pipe/main.go`

### Task 6: Update Tests

- Rename all test function references
- Verify all tests pass

## API Changes Summary

| Before | After | Cardinality |
|--------|-------|-------------|
| `Transform(in, func(In) Out)` | `Map(in, func(In) Out)` | 1:1 |
| `Process(in, func(In) []Out)` | `Expand(in, func(In) []Out)` | 1:N |
| `Filter(in, func(T) bool)` | `Filter(in, func(T) bool)` | 1:0/1 (unchanged) |
| `Sink(in, func(T))` | `Drain(in, func(T))` | consume with effect |
| `Drain(in)` | `Drop(in)` | discard all |

## Final API (After Refactoring)

### Sources
```go
FromSlice[T](slice []T) <-chan T
FromValues[T](values ...T) <-chan T
FromRange(i ...int) <-chan int
FromFunc[T](ctx, func() T) <-chan T
```

### Transforms
```go
Filter[T](in, func(T) bool) <-chan T           // 1:0/1 predicate
Map[In, Out](in, func(In) Out) <-chan Out      // 1:1 pure transform
Expand[In, Out](in, func(In) []Out) <-chan Out // 1:N pure expand
Flatten[T](in <-chan []T) <-chan T             // unbatch slices
Buffer[T](in, size int) <-chan T               // add buffering
```

### Sinks
```go
Drain[T](in, func(T)) <-chan struct{}  // consume with side effect
Drop[T](in) <-chan struct{}            // discard all items
ToSlice[T](in) []T                     // collect to slice (blocking)
```

### Batching
```go
Collect[T](in, maxSize, maxDuration) <-chan []T
Batch[In, Out](in, func([]In) []Out, maxSize, maxDuration) <-chan Out
GroupBy[V, K](in, keyFunc, config) <-chan Group[K, V]
```

### Fan-In/Out
```go
Merge[T](ins ...<-chan T) <-chan T
Broadcast[T](in, n int) []<-chan T
Route[T](in, func(T) int, n int) []<-chan T
```

### Control
```go
Cancel[T](ctx, in, func(T, error)) <-chan T
```

## Acceptance Criteria

- [ ] `Transform` renamed to `Map`
- [ ] `Process` renamed to `Expand`
- [ ] `Sink` renamed to `Drain`
- [ ] `Drain` renamed to `Drop`
- [ ] All test files renamed and updated
- [ ] `channel/doc.go` updated
- [ ] `README.md` examples updated
- [ ] `AGENTS.md` guidance updated
- [ ] Examples updated
- [ ] All tests pass
- [ ] Build passes (`make build && make vet`)

## Semantic Clarity

After refactoring, the naming clearly communicates:

| Function | Semantic Meaning |
|----------|------------------|
| `Map` | Transform each item 1:1 (like mathematical map) |
| `Expand` | Expand each item to 0+ items (1:N) |
| `Filter` | Keep items matching predicate |
| `Drain` | Consume channel, apply side effect to each item |
| `Drop` | Consume channel, discard all items |
| `Flatten` | Unpack slices into individual items |

The distinction between `Drain` and `Drop` is now explicit:
- **Drain**: "I want to process each item" (with handler)
- **Drop**: "I want to discard all items" (no handler)

Both return done signals for graceful shutdown consistency.
