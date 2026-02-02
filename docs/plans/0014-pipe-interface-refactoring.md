# Plan 0014: Pipe Interface Refactoring

**Status:** Proposed

## Overview

Refactor the pipe package to introduce semantic interfaces (`Processor`, `Source`, `BatchProcessor`, `Filter`, `Mapper`, `Expander`, `Sink`) alongside the composition interface (`Pipe`). Concrete structs use `*Pipe` suffix and implement both their semantic interface and the `Pipe` interface. An internal base type reduces code duplication.

Key distinction: **Pure** operations (Filter, Mapper, Expander) have no context or error returns, while **Impure** operations (Processor, Source, BatchProcessor, Sink) include context and error handling for I/O, external calls, or fallible operations.

## Goals

1. Clearer type semantics - interfaces describe component capabilities
2. Better API ergonomics - `Source[Out]` cleaner than `Pipe[struct{}, Out]`
3. Direct invocation - call `Process()` or `Source()` without channel setup
4. Composition via `Pipe` - all components still work with `Apply()`
5. Consistent naming - interface is concept, struct has `*Pipe` suffix
6. Reduced duplication - internal base type for shared lifecycle management
7. Purity distinction - pure transformations (no ctx/error) vs impure operations (with ctx/error)

## Proposed Design

### Interfaces

```go
// ============================================
// COMPOSITION INTERFACE (unchanged)
// ============================================

// Pipe is the universal interface for pipeline composition.
// All pipe types implement this to enable Apply() chaining.
type Pipe[In, Out any] interface {
    Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}

// ============================================
// PURE SEMANTIC INTERFACES (no ctx, no error)
// ============================================

// Filter selectively passes items (1:0/1 cardinality).
// Pure predicate function - no side effects.
type Filter[T any] interface {
    Filter(in T) bool
}

// Mapper transforms individual items (1:1 cardinality).
// Pure transformation - no side effects.
type Mapper[In, Out any] interface {
    Map(in In) Out
}

// Expander transforms items to zero or more outputs (1:N cardinality).
// Pure transformation - no side effects.
type Expander[In, Out any] interface {
    Expand(in In) []Out
}

// ============================================
// IMPURE SEMANTIC INTERFACES (with ctx, error)
// ============================================

// Source produces values on demand.
// Impure - may involve I/O or external state.
type Source[Out any] interface {
    Source(ctx context.Context) ([]Out, error)  // single batch
}

// Processor transforms individual items with potential side effects (0:N cardinality).
// Impure - may involve I/O, external calls, or fallible operations.
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

// BatchProcessor transforms batches of items.
// Impure - may involve I/O or external state.
type BatchProcessor[In, Out any] interface {
    ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
}

// Sink consumes items without producing output.
// Impure - typically involves I/O or external state.
type Sink[In any] interface {
    Sink(ctx context.Context, in In) error
}
```

### Internal Base Type

```go
// pipe/base.go (unexported)

// pipe is the internal base type for shared lifecycle management.
type pipe[In, Out any] struct {
    cfg     Config
    mw      []middleware.Middleware[In, Out]
    mu      sync.Mutex
    started bool
}

func (p *pipe[In, Out]) use(mw ...middleware.Middleware[In, Out]) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.started {
        return ErrAlreadyStarted
    }
    p.mw = append(p.mw, mw...)
    return nil
}

func (p *pipe[In, Out]) start() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.started {
        return ErrAlreadyStarted
    }
    p.started = true
    return nil
}
```

### Concrete Types

```go
// SourcePipe implements Source and Pipe[struct{}, Out]
type SourcePipe[Out any] struct {
    pipe[struct{}, Out]
    handle SourceFunc[Out]
}

// Source produces a single batch (semantic interface).
func (s *SourcePipe[Out]) Source(ctx context.Context) ([]Out, error)

// Pipe implements Pipe interface - triggered by input channel.
// Each struct{} received triggers one Source() call.
func (s *SourcePipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error)


// ProcessorPipe implements Processor and Pipe[In, Out]
type ProcessorPipe[In, Out any] struct {
    pipe[In, Out]
    handle ProcessFunc[In, Out]
}

func (p *ProcessorPipe[In, Out]) Process(ctx context.Context, in In) ([]Out, error)
func (p *ProcessorPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)


// BatchProcessorPipe implements BatchProcessor and Pipe[In, Out]
type BatchProcessorPipe[In, Out any] struct {
    pipe[In, Out]
    handle    BatchFunc[In, Out]
    batchCfg  BatchConfig
}

func (b *BatchProcessorPipe[In, Out]) ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
func (b *BatchProcessorPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)


// FilterPipe implements Filter and Pipe[T, T]
type FilterPipe[T any] struct {
    pipe[T, T]
    handle FilterFunc[T]
}

func (f *FilterPipe[T]) Filter(in T) bool  // pure - no ctx, no error
func (f *FilterPipe[T]) Pipe(ctx context.Context, in <-chan T) (<-chan T, error)


// MapperPipe implements Mapper and Pipe[In, Out]
type MapperPipe[In, Out any] struct {
    pipe[In, Out]
    handle MapFunc[In, Out]
}

func (m *MapperPipe[In, Out]) Map(in In) Out  // pure - no ctx, no error
func (m *MapperPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)


// ExpanderPipe implements Expander and Pipe[In, Out]
type ExpanderPipe[In, Out any] struct {
    pipe[In, Out]
    handle ExpandFunc[In, Out]
}

func (e *ExpanderPipe[In, Out]) Expand(in In) []Out  // pure - no ctx, no error
func (e *ExpanderPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)


// SinkPipe implements Sink and Pipe[In, struct{}]
type SinkPipe[In any] struct {
    pipe[In, struct{}]
    handle SinkFunc[In]
}

func (s *SinkPipe[In]) Sink(ctx context.Context, in In) error
func (s *SinkPipe[In]) Pipe(ctx context.Context, in <-chan In) (<-chan struct{}, error)
```

### Constructors

```go
// Impure constructors (operations with ctx and error)
func NewSource[Out any](fn SourceFunc[Out], cfg Config) *SourcePipe[Out]
func NewProcessor[In, Out any](fn ProcessFunc[In, Out], cfg Config) *ProcessorPipe[In, Out]
func NewBatchProcessor[In, Out any](fn BatchFunc[In, Out], cfg BatchConfig) *BatchProcessorPipe[In, Out]
func NewSink[In any](fn SinkFunc[In], cfg Config) *SinkPipe[In]

// Pure constructors (operations without ctx or error)
func NewFilter[T any](fn FilterFunc[T], cfg Config) *FilterPipe[T]
func NewMapper[In, Out any](fn MapFunc[In, Out], cfg Config) *MapperPipe[In, Out]
func NewExpander[In, Out any](fn ExpandFunc[In, Out], cfg Config) *ExpanderPipe[In, Out]
```

### Function Type Aliases

```go
// Impure function types (with ctx and error)
type SourceFunc[Out any] func(ctx context.Context) ([]Out, error)
type ProcessFunc[In, Out any] func(ctx context.Context, in In) ([]Out, error)
type BatchFunc[In, Out any] func(ctx context.Context, batch []In) ([]Out, error)
type SinkFunc[In any] func(ctx context.Context, in In) error

// Pure function types (no ctx, no error)
type FilterFunc[T any] func(in T) bool
type MapFunc[In, Out any] func(in In) Out
type ExpandFunc[In, Out any] func(in In) []Out
```

## Design Decisions (Resolved)

### 1. Struct naming uses `*Pipe` suffix
- Interface: `Source` → Struct: `SourcePipe`
- Interface: `Processor` → Struct: `ProcessorPipe`
- Interface: `BatchProcessor` → Struct: `BatchProcessorPipe`
- Interface: `Filter` → Struct: `FilterPipe`
- Interface: `Mapper` → Struct: `MapperPipe`
- Interface: `Expander` → Struct: `ExpanderPipe`
- Interface: `Sink` → Struct: `SinkPipe`

### 2. SourcePipe.Pipe() uses input as trigger
Input channel is NOT ignored. Each `struct{}` received triggers one `Source()` call. This enables controlled generation in pipelines.

### 3. No Consume method on SinkPipe
`Consume()` would be identical to `Pipe()` - redundant. Users just call `Pipe()`.

### 4. Direct methods don't apply middleware
`Process()`, `Sink()`, `Source()`, `ProcessBatch()` call the raw handler function. Middleware only applies through `Pipe()`. This keeps direct methods simple, predictable, and useful for testing.

### 5. Internal base type for shared lifecycle
Unexported `pipe[In, Out]` struct handles:
- Config storage
- Middleware collection
- Started state with mutex
- `use()` and `start()` methods

Reduces duplication across ProcessorPipe, SinkPipe, SourcePipe, BatchProcessorPipe.

### 6. Filter/Mapper/Expander are pure operations
For consistency, `Filter`, `Mapper`, and `Expander` have their own interfaces and `*Pipe` structs:
- `Filter[T]` interface with `Filter(T) bool` → `FilterPipe[T]` struct
- `Mapper[In, Out]` interface with `Map(In) Out` → `MapperPipe[In, Out]` struct
- `Expander[In, Out]` interface with `Expand(In) []Out` → `ExpanderPipe[In, Out]` struct

These are **pure** operations - no context, no error returns. This distinguishes them from `Processor` which is impure (has context and error for I/O operations).

Naming follows Go `-er` convention: Mapper (like Reader/Writer), Expander (1:N expansion).

### 7. Merger/Distributor stay as infrastructure
They are fan-in/fan-out primitives, not pipeline stages. No semantic interfaces needed.

## Producer and Trigger Design

### Conceptual Model

```
┌─────────────────────────────────────────────────────────────┐
│                        Producer                              │
│                                                              │
│   ┌─────────────┐         ┌─────────────────────────────┐   │
│   │   Trigger   │ ──────► │     Pipe[struct{}, Out]     │   │
│   │ TriggerFunc │         │  (e.g., SourcePipe)         │   │
│   └─────────────┘         └─────────────────────────────┘   │
│                                                              │
│   Produce(ctx) (<-chan Out, error)                          │
└─────────────────────────────────────────────────────────────┘
```

**Separation of concerns:**
- **Source** = what to produce (the data)
- **Trigger** = when to produce (the timing)
- **Producer** = autonomous streaming (combines trigger + pipe)

### Core Types (in `pipe` package)

```go
// TriggerFunc produces trigger signals until context is cancelled.
// Context is passed at runtime by Producer - lifecycle is external concern.
type TriggerFunc func(ctx context.Context) <-chan struct{}

// Producer provides autonomous streaming capability.
type Producer[Out any] interface {
    Produce(ctx context.Context) (<-chan Out, error)
}

// NewProducer combines a trigger with any Pipe[struct{}, Out].
func NewProducer[Out any](trigger TriggerFunc, p Pipe[struct{}, Out]) Producer[Out]
```

### Trigger Subpackage (`pipe/trigger`)

Factory functions return `pipe.TriggerFunc`:

```go
// pipe/trigger/trigger.go
package trigger

import "github.com/fxsml/gopipe/pipe"

// Infinite triggers as fast as possible.
func Infinite() pipe.TriggerFunc

// Ticker triggers at regular intervals.
func Ticker(d time.Duration) pipe.TriggerFunc

// Count triggers exactly n times.
func Count(n int) pipe.TriggerFunc

// Once triggers exactly once.
func Once() pipe.TriggerFunc
```

### Usage Examples

```go
import (
    "github.com/fxsml/gopipe/pipe"
    "github.com/fxsml/gopipe/pipe/trigger"
)

// Create source (no autonomous streaming capability)
src := pipe.NewSource(fn, cfg)

// Use with explicit trigger via Pipe()
src.Pipe(ctx, triggerChan)

// Or wrap as Producer for autonomous operation
producer := pipe.NewProducer(trigger.Infinite(), src)
out, _ := producer.Produce(ctx)

// With ticker
producer := pipe.NewProducer(trigger.Ticker(time.Second), src)

// Producer works with any Pipe[struct{}, Out], including pipelines
pipeline := pipe.Apply(src, mapper, filter)
producer := pipe.NewProducer(trigger.Ticker(time.Second), pipeline)
```

### Design Decisions

1. **SourcePipe has no Produce() method** - Keep it simple. Producer is a separate composition.
2. **TriggerFunc defined in `pipe` package** - Core type alongside other func types.
3. **Trigger implementations in `trigger` subpackage** - Mirrors `middleware` pattern.
4. **Factory functions for triggers** - `trigger.Infinite()`, `trigger.Ticker(d)` - consistent, allows params.
5. **Context passed at runtime** - TriggerFunc receives ctx from Producer, not at construction.

## Tasks

### Task 1: Create Internal Base Type

**Files to Create:**
- `pipe/base.go` - Internal `pipe[In, Out]` struct

**Changes:**
```go
type pipe[In, Out any] struct {
    cfg     Config
    mw      []middleware.Middleware[In, Out]
    mu      sync.Mutex
    started bool
}

func (p *pipe[In, Out]) use(mw ...middleware.Middleware[In, Out]) error
func (p *pipe[In, Out]) start() error
```

### Task 2: Add Semantic Interfaces

**Files to Modify:**
- `pipe/pipe.go` - Add `Processor`, `BatchProcessor`, `Sink` interfaces
- `pipe/source.go` - Add `Source` interface

### Task 3: Rename ProcessPipe to ProcessorPipe

**Files to Modify:**
- `pipe/pipe.go` - Rename struct, embed base type, add `Process()` method

### Task 4: Rename BatchPipe to BatchProcessorPipe

**Files to Modify:**
- `pipe/pipe.go` - Rename struct, embed base type, add `ProcessBatch()` method

### Task 5: Rename GeneratePipe to SourcePipe

**Files to Modify:**
- `pipe/source.go` (rename from `pipe/generator.go`) - Rename struct, embed base type
- Update `Source()` to return single batch
- Add `Pipe()` with trigger semantics

### Task 6: Create FilterPipe

**Files to Create:**
- `pipe/filter.go` - Create `Filter` interface and `FilterPipe` struct

```go
type Filter[T any] interface {
    Filter(in T) bool  // pure - no ctx, no error
}

type FilterFunc[T any] func(in T) bool

type FilterPipe[T any] struct {
    pipe[T, T]
    handle FilterFunc[T]
}

func NewFilter[T any](fn FilterFunc[T], cfg Config) *FilterPipe[T]
func (f *FilterPipe[T]) Filter(in T) bool  // direct call
func (f *FilterPipe[T]) Pipe(ctx, <-chan T) (<-chan T, error)
```

### Task 7: Create MapperPipe

**Files to Create:**
- `pipe/mapper.go` - Create `Mapper` interface and `MapperPipe` struct

```go
type Mapper[In, Out any] interface {
    Map(in In) Out  // pure - no ctx, no error
}

type MapFunc[In, Out any] func(in In) Out

type MapperPipe[In, Out any] struct {
    pipe[In, Out]
    handle MapFunc[In, Out]
}

func NewMapper[In, Out any](fn MapFunc[In, Out], cfg Config) *MapperPipe[In, Out]
func (m *MapperPipe[In, Out]) Map(in In) Out  // direct call
func (m *MapperPipe[In, Out]) Pipe(ctx, <-chan In) (<-chan Out, error)
```

### Task 8: Create ExpanderPipe

**Files to Create:**
- `pipe/expander.go` - Create `Expander` interface and `ExpanderPipe` struct

```go
type Expander[In, Out any] interface {
    Expand(in In) []Out  // pure - no ctx, no error
}

type ExpandFunc[In, Out any] func(in In) []Out

type ExpanderPipe[In, Out any] struct {
    pipe[In, Out]
    handle ExpandFunc[In, Out]
}

func NewExpander[In, Out any](fn ExpandFunc[In, Out], cfg Config) *ExpanderPipe[In, Out]
func (e *ExpanderPipe[In, Out]) Expand(in In) []Out  // direct call
func (e *ExpanderPipe[In, Out]) Pipe(ctx, <-chan In) (<-chan Out, error)
```

### Task 9: Update SinkPipe

**Files to Modify:**
- `pipe/pipe.go` - Create proper `SinkPipe` struct with base type, add `Sink()` method

### Task 10: Add Function Type Aliases

**Files to Modify:**
- `pipe/processing.go` - Add `SourceFunc`, `BatchFunc`, `SinkFunc`, `FilterFunc`, `MapFunc`, `ExpandFunc`

### Task 11: Rename Constructors

| Before | After |
|--------|-------|
| `NewProcessPipe` | `NewProcessor` |
| `NewBatchPipe` | `NewBatchProcessor` |
| `NewFilterPipe` | `NewFilter` |
| `NewTransformPipe` | `NewMapper` |
| (new) | `NewExpander` |
| `NewSinkPipe` | `NewSink` |
| `NewGenerator` | `NewSource` |

### Task 12: Update Tests

- Rename all test references
- Add tests for new semantic methods
- Add tests for Pipe() trigger behavior on SourcePipe
- Add tests for pure vs impure operations
- Verify middleware NOT applied to direct methods

### Task 13: Update message Package

- `message/pipes.go`
- `message/router.go`
- `message/cloudevents/publisher.go`
- `message/cloudevents/subscriber.go`

### Task 14: Add TriggerFunc and Producer

**Files to Modify:**
- `pipe/pipe.go` - Add `TriggerFunc` type and `Producer` interface
- `pipe/producer.go` - Add `NewProducer` and internal producer struct

```go
// pipe/pipe.go
type TriggerFunc func(ctx context.Context) <-chan struct{}

type Producer[Out any] interface {
    Produce(ctx context.Context) (<-chan Out, error)
}

// pipe/producer.go
func NewProducer[Out any](trigger TriggerFunc, p Pipe[struct{}, Out]) Producer[Out]
```

### Task 15: Create trigger Subpackage

**Files to Create:**
- `pipe/trigger/trigger.go` - Factory functions
- `pipe/trigger/trigger_test.go` - Tests

```go
func Infinite() pipe.TriggerFunc
func Ticker(d time.Duration) pipe.TriggerFunc
func Count(n int) pipe.TriggerFunc
func Once() pipe.TriggerFunc
```

### Task 16: Update Documentation

- `pipe/doc.go`
- `README.md`
- Create ADR

## API Changes Summary

### Breaking Changes

| Before | After |
|--------|-------|
| `ProcessPipe` | `ProcessorPipe` |
| `BatchPipe` | `BatchProcessorPipe` |
| `GeneratePipe` | `SourcePipe` |
| `NewProcessPipe` | `NewProcessor` |
| `NewBatchPipe` | `NewBatchProcessor` |
| `NewFilterPipe` | `NewFilter` |
| `NewTransformPipe` | `NewMapper` |
| (new) | `NewExpander` |
| `NewSinkPipe` | `NewSink` |
| `NewGenerator` | `NewSource` |
| `NewFilterPipe() -> *ProcessPipe` | `NewFilter() -> *FilterPipe` |
| `NewTransformPipe() -> *ProcessPipe` | `NewMapper() -> *MapperPipe` |
| (new) | `NewExpander() -> *ExpanderPipe` |
| `FilterFunc(ctx, T) (bool, error)` | `FilterFunc(T) bool` (pure) |
| `TransformFunc(ctx, In) (Out, error)` | `MapFunc(In) Out` (pure) |
| (new) | `ExpandFunc(In) []Out` (pure) |
| `GenerateFunc` | `SourceFunc` |
| `GeneratePipe.Generate()` returns channel | `SourcePipe.Source()` returns single batch |

### New APIs

| API | Description |
|-----|-------------|
| `Source[Out]` interface | Impure semantic interface with `Source(ctx) ([]Out, error)` |
| `Processor[In, Out]` interface | Impure semantic interface with `Process(ctx, In) ([]Out, error)` |
| `BatchProcessor[In, Out]` interface | Impure semantic interface with `ProcessBatch()` |
| `Filter[T]` interface | Pure semantic interface with `Filter(T) bool` |
| `Mapper[In, Out]` interface | Pure semantic interface with `Map(In) Out` |
| `Expander[In, Out]` interface | Pure semantic interface with `Expand(In) []Out` |
| `Sink[In]` interface | Impure semantic interface with `Sink()` |
| `SourcePipe.Source()` | Single batch generation (impure) |
| `SourcePipe.Pipe()` | Triggered streaming (input as trigger) |
| `ProcessorPipe.Process()` | Direct single-item invocation (impure) |
| `BatchProcessorPipe.ProcessBatch()` | Direct batch invocation (impure) |
| `FilterPipe[T]` struct | Implements `Filter` and `Pipe[T, T]` |
| `FilterPipe.Filter()` | Direct single-item filtering (pure) |
| `MapperPipe[In, Out]` struct | Implements `Mapper` and `Pipe[In, Out]` |
| `MapperPipe.Map()` | Direct 1:1 transformation (pure) |
| `ExpanderPipe[In, Out]` struct | Implements `Expander` and `Pipe[In, Out]` |
| `ExpanderPipe.Expand()` | Direct 1:N transformation (pure) |
| `SinkPipe.Sink()` | Direct single-item consumption |
| `TriggerFunc` | Type for trigger functions |
| `Producer[Out]` interface | Autonomous streaming with `Produce()` |
| `NewProducer()` | Combines trigger + Pipe[struct{}, Out] |
| `trigger.Infinite()` | Trigger as fast as possible |
| `trigger.Ticker(d)` | Trigger at intervals |
| `trigger.Count(n)` | Trigger exactly n times |
| `trigger.Once()` | Trigger exactly once |

## Acceptance Criteria

- [ ] Internal base type created
- [ ] All semantic interfaces defined (Source, Processor, BatchProcessor, Filter, Mapper, Expander, Sink)
- [ ] All structs renamed with `*Pipe` suffix
- [ ] All structs implement their semantic interface
- [ ] FilterPipe created with pure Filter(T) bool method
- [ ] MapperPipe created with pure Map(In) Out method
- [ ] ExpanderPipe created with pure Expand(In) []Out method
- [ ] SourcePipe.Pipe() uses input as trigger
- [ ] SourcePipe.Source() returns single batch (not channel)
- [ ] All constructors renamed (NewSource, NewMapper, NewExpander, etc.)
- [ ] Direct methods don't apply middleware
- [ ] TriggerFunc type defined in pipe package
- [ ] Producer interface and NewProducer implemented
- [ ] trigger subpackage created with factory functions
- [ ] All pipe package tests pass
- [ ] All message package tests pass
- [ ] Build passes (`make build && make vet`)
- [ ] Documentation updated

## Outlook (Future Work)

The following ideas were discussed but are **not part of this refactoring**. They may be implemented in separate future work.

### 1. Pre/Post Mapping for Router

Add `PreMap` and `PostMap` fields to `Router` for type conversion before and after handler processing:

```go
type RouterConfig struct {
    // ... existing fields ...
    PreMap  func(*Message) *TypedMessage   // optional
    PostMap func(*TypedMessage) *Message   // optional
}
```

This would allow handlers to work with domain-specific types while Router handles conversion. Naming options considered:
- `PreMap`/`PostMap` (Go-idiomatic)
- `PreTransform`/`PostTransform`
- `ToHandler`/`FromHandler`
- `PreProcess`/`PostProcess`

Decision deferred - middleware may be sufficient for enrichment/normalization use cases.

### 2. Unified Process() Method via Type Embedding

Add `Process()` method to all pipes via composition, allowing any pipe to be used where `Processor` interface is expected:

```go
// Hypothetical - wrapping pure functions
func (f *FilterPipe[T]) Process(ctx context.Context, in T) ([]T, error) {
    if f.Filter(in) {
        return []T{in}, nil
    }
    return nil, nil
}
```

Decided against this due to:
- Array return overhead for pure operations (wrapping single value in `[]Out{out}`)
- Mixing pure/impure semantics
- Unclear benefit over explicit composition

### 3. Conditional/Branching Pipes

Support for branching pipelines based on conditions:

```go
// Route to different pipes based on predicate
branch := pipe.NewBranch(predicate, truePipe, falsePipe)
```

### 4. Error Recovery Pipes

Pipes that can recover from errors and continue processing:

```go
// On error, call recovery function instead of failing
recovery := pipe.NewRecover(mainPipe, func(in In, err error) ([]Out, error) {
    // handle error, possibly return fallback value
})
```
