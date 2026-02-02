# Plan 0008: Pipe Interface Refactoring

**Status:** Proposed

## Overview

Refactor the pipe package to introduce semantic interfaces (`Processor`, `Generator`, `BatchProcessor`, `Sink`) alongside the composition interface (`Pipe`). Concrete structs use `*Pipe` suffix and implement both their semantic interface and the `Pipe` interface. An internal base type reduces code duplication.

## Goals

1. Clearer type semantics - interfaces describe component capabilities
2. Better API ergonomics - `Generator[Out]` cleaner than `Pipe[struct{}, Out]`
3. Direct invocation - call `Process()` or `Generate()` without channel setup
4. Composition via `Pipe` - all components still work with `Apply()`
5. Consistent naming - interface is concept, struct has `*Pipe` suffix
6. Reduced duplication - internal base type for shared lifecycle management

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
// SEMANTIC INTERFACES
// ============================================

// Generator produces values on demand.
type Generator[Out any] interface {
    Generate(ctx context.Context) ([]Out, error)  // single batch
}

// Processor transforms individual items.
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

// BatchProcessor transforms batches of items.
type BatchProcessor[In, Out any] interface {
    ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
}

// Sink consumes items without producing output.
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
// GeneratorPipe implements Generator and Pipe[struct{}, Out]
type GeneratorPipe[Out any] struct {
    pipe[struct{}, Out]
    handle GenerateFunc[Out]
}

// Generate produces a single batch (semantic interface).
func (g *GeneratorPipe[Out]) Generate(ctx context.Context) ([]Out, error)

// Produce starts autonomous generation (internal trigger channel).
// TBD: May be renamed to Subscribe or other name.
func (g *GeneratorPipe[Out]) Produce(ctx context.Context) (<-chan Out, error)

// Pipe implements Pipe interface - triggered by input channel.
// Each struct{} received triggers one Generate() call.
func (g *GeneratorPipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error)


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
func NewGenerator[Out any](fn GenerateFunc[Out], cfg Config) *GeneratorPipe[Out]
func NewProcessor[In, Out any](fn ProcessFunc[In, Out], cfg Config) *ProcessorPipe[In, Out]
func NewBatchProcessor[In, Out any](fn BatchFunc[In, Out], cfg BatchConfig) *BatchProcessorPipe[In, Out]
func NewSink[In any](fn SinkFunc[In], cfg Config) *SinkPipe[In]

// Helpers - return ProcessorPipe with adapted signatures
func NewFilter[T any](fn FilterFunc[T], cfg Config) *ProcessorPipe[T, T]
func NewTransform[In, Out any](fn TransformFunc[In, Out], cfg Config) *ProcessorPipe[In, Out]
```

### Function Type Aliases

```go
type GenerateFunc[Out any] func(ctx context.Context) ([]Out, error)
type ProcessFunc[In, Out any] func(ctx context.Context, in In) ([]Out, error)
type BatchFunc[In, Out any] func(ctx context.Context, batch []In) ([]Out, error)
type SinkFunc[In any] func(ctx context.Context, in In) error
type FilterFunc[T any] func(ctx context.Context, in T) (bool, error)
type TransformFunc[In, Out any] func(ctx context.Context, in In) (Out, error)
```

## Design Decisions (Resolved)

### 1. Struct naming uses `*Pipe` suffix
- Interface: `Processor` → Struct: `ProcessorPipe`
- Interface: `Generator` → Struct: `GeneratorPipe`
- Interface: `BatchProcessor` → Struct: `BatchProcessorPipe`
- Interface: `Sink` → Struct: `SinkPipe`

### 2. GeneratorPipe.Pipe() uses input as trigger
Input channel is NOT ignored. Each `struct{}` received triggers one `Generate()` call. This enables controlled generation in pipelines.

### 3. GeneratorPipe.Produce() is autonomous
`Produce()` creates an internal trigger channel for convenience (self-triggering continuous generation). This is the current `Generate()` behavior.

**Naming TBD:** `Produce` vs `Subscribe` vs other options - see Open Questions.

### 4. No Consume method on SinkPipe
`Consume()` would be identical to `Pipe()` - redundant. Users just call `Pipe()`.

### 5. Direct methods don't apply middleware
`Process()`, `Sink()`, `Generate()`, `ProcessBatch()` call the raw handler function. Middleware only applies through `Pipe()`. This keeps direct methods simple, predictable, and useful for testing.

### 6. Internal base type for shared lifecycle
Unexported `pipe[In, Out]` struct handles:
- Config storage
- Middleware collection
- Started state with mutex
- `use()` and `start()` methods

Reduces duplication across ProcessorPipe, SinkPipe, GeneratorPipe, BatchProcessorPipe.

### 7. Filter/Transform are constructor helpers only
`NewFilter` and `NewTransform` return `*ProcessorPipe` with adapted function signatures. No separate types needed.

### 8. Merger/Distributor stay as infrastructure
They are fan-in/fan-out primitives, not pipeline stages. No semantic interfaces needed.

## Producer and Trigger Design

### Conceptual Model

```
┌─────────────────────────────────────────────────────────────┐
│                        Producer                              │
│                                                              │
│   ┌─────────────┐         ┌─────────────────────────────┐   │
│   │   Trigger   │ ──────► │     Pipe[struct{}, Out]     │   │
│   │ TriggerFunc │         │  (e.g., GeneratorPipe)      │   │
│   └─────────────┘         └─────────────────────────────┘   │
│                                                              │
│   Produce(ctx) (<-chan Out, error)                          │
└─────────────────────────────────────────────────────────────┘
```

**Separation of concerns:**
- **Generator** = what to produce (the data)
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

// Create generator (no autonomous streaming capability)
gen := pipe.NewGenerator(fn, cfg)

// Use with explicit trigger via Pipe()
gen.Pipe(ctx, triggerChan)

// Or wrap as Producer for autonomous operation
producer := pipe.NewProducer(trigger.Infinite(), gen)
out, _ := producer.Produce(ctx)

// With ticker
producer := pipe.NewProducer(trigger.Ticker(time.Second), gen)

// Producer works with any Pipe[struct{}, Out], including pipelines
pipeline := pipe.Apply(gen, transform, filter)
producer := pipe.NewProducer(trigger.Ticker(time.Second), pipeline)
```

### Design Decisions

1. **GeneratorPipe has no Produce() method** - Keep it simple. Producer is a separate composition.
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
- `pipe/generator.go` - Update `Generator` interface (now returns single batch)

### Task 3: Rename ProcessPipe to ProcessorPipe

**Files to Modify:**
- `pipe/pipe.go` - Rename struct, embed base type, add `Process()` method

### Task 4: Rename BatchPipe to BatchProcessorPipe

**Files to Modify:**
- `pipe/pipe.go` - Rename struct, embed base type, add `ProcessBatch()` method

### Task 5: Rename GeneratePipe to GeneratorPipe

**Files to Modify:**
- `pipe/generator.go` - Rename struct, embed base type
- Update `Generate()` to return single batch
- Add `Produce()` for autonomous streaming
- Add `Pipe()` with trigger semantics

### Task 6: Update SinkPipe

**Files to Modify:**
- `pipe/pipe.go` - Create proper `SinkPipe` struct with base type, add `Sink()` method

### Task 7: Add Function Type Aliases

**Files to Modify:**
- `pipe/processing.go` - Add `GenerateFunc`, `BatchFunc`, `SinkFunc`, `FilterFunc`, `TransformFunc`

### Task 8: Rename Constructors

| Before | After |
|--------|-------|
| `NewProcessPipe` | `NewProcessor` |
| `NewBatchPipe` | `NewBatchProcessor` |
| `NewFilterPipe` | `NewFilter` |
| `NewTransformPipe` | `NewTransform` |
| `NewSinkPipe` | `NewSink` |
| `NewGenerator` | `NewGenerator` (unchanged) |

### Task 9: Update Tests

- Rename all test references
- Add tests for new semantic methods
- Add tests for Pipe() trigger behavior on GeneratorPipe
- Verify middleware NOT applied to direct methods

### Task 10: Update message Package

- `message/pipes.go`
- `message/router.go`
- `message/cloudevents/publisher.go`
- `message/cloudevents/subscriber.go`

### Task 11: Add TriggerFunc and Producer

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

### Task 12: Create trigger Subpackage

**Files to Create:**
- `pipe/trigger/trigger.go` - Factory functions
- `pipe/trigger/trigger_test.go` - Tests

```go
func Infinite() pipe.TriggerFunc
func Ticker(d time.Duration) pipe.TriggerFunc
func Count(n int) pipe.TriggerFunc
func Once() pipe.TriggerFunc
```

### Task 13: Update message Package

- `message/pipes.go`
- `message/router.go`
- `message/cloudevents/publisher.go`
- `message/cloudevents/subscriber.go`

### Task 14: Update Documentation

- `pipe/doc.go`
- `README.md`
- Create ADR

## API Changes Summary

### Breaking Changes

| Before | After |
|--------|-------|
| `ProcessPipe` | `ProcessorPipe` |
| `BatchPipe` | `BatchProcessorPipe` |
| `GeneratePipe` | `GeneratorPipe` |
| `NewProcessPipe` | `NewProcessor` |
| `NewBatchPipe` | `NewBatchProcessor` |
| `NewFilterPipe` | `NewFilter` |
| `NewTransformPipe` | `NewTransform` |
| `NewSinkPipe` | `NewSink` |
| `GeneratePipe.Generate()` returns channel | `GeneratorPipe.Generate()` returns single batch |

### New APIs

| API | Description |
|-----|-------------|
| `Processor[In, Out]` interface | Semantic interface with `Process()` |
| `BatchProcessor[In, Out]` interface | Semantic interface with `ProcessBatch()` |
| `Sink[In]` interface | Semantic interface with `Sink()` |
| `ProcessorPipe.Process()` | Direct single-item invocation |
| `BatchProcessorPipe.ProcessBatch()` | Direct batch invocation |
| `GeneratorPipe.Generate()` | Single batch generation |
| `GeneratorPipe.Pipe()` | Triggered streaming (input as trigger) |
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
- [ ] All semantic interfaces defined (Processor, BatchProcessor, Sink, Generator)
- [ ] All structs renamed with `*Pipe` suffix
- [ ] All structs implement their semantic interface
- [ ] GeneratorPipe.Pipe() uses input as trigger
- [ ] GeneratorPipe.Generate() returns single batch (not channel)
- [ ] All constructors renamed
- [ ] Direct methods don't apply middleware
- [ ] TriggerFunc type defined in pipe package
- [ ] Producer interface and NewProducer implemented
- [ ] trigger subpackage created with factory functions
- [ ] All pipe package tests pass
- [ ] All message package tests pass
- [ ] Build passes (`make build && make vet`)
- [ ] Documentation updated
