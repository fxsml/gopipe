# Plan 0008: Pipe Interface Refactoring

**Status:** Proposed

## Overview

Refactor the pipe package to introduce semantic interfaces (`Processor`, `Generator`, `BatchProcessor`, `Sink`) alongside the composition interface (`Pipe`). Each concrete struct (`ProcessPipe`, `GeneratePipe`, `BatchPipe`, `SinkPipe`) implements both its semantic interface and the `Pipe` interface.

## Goals

1. Clearer type semantics - interfaces describe component capabilities
2. Better API ergonomics - `Generator[Out]` cleaner than `Pipe[struct{}, Out]`
3. Direct invocation - call `Process()` or `Generate()` without channel setup
4. Composition via `Pipe` - all components still work with `Apply()`
5. Consistent naming - interface is concept, struct is `*Pipe` implementation

## Current State

```go
// Single interface for composition
type Pipe[In, Out any] interface {
    Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)
}

// Generator already has its own interface (partial implementation of this pattern)
type Generator[Out any] interface {
    Generate(ctx context.Context) (<-chan Out, error)
}

// Concrete types
type ProcessPipe[In, Out any] struct { ... }  // implements Pipe
type BatchPipe[In, Out any] struct { ... }    // implements Pipe
type GeneratePipe[Out any] struct { ... }     // implements Generator (not Pipe!)
// No SinkPipe - NewSinkPipe returns *ProcessPipe[In, struct{}]

// Constructors
func NewProcessPipe(...) *ProcessPipe
func NewBatchPipe(...) *BatchPipe
func NewGenerator(...) *GeneratePipe
func NewFilterPipe(...) *ProcessPipe  // helper
func NewTransformPipe(...) *ProcessPipe  // helper
func NewSinkPipe(...) *ProcessPipe  // helper, returns ProcessPipe not SinkPipe
```

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

// Generator produces a stream of values without input.
type Generator[Out any] interface {
    Generate(ctx context.Context) (<-chan Out, error)
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

### Concrete Types

```go
// GeneratePipe implements Generator and Pipe[struct{}, Out]
type GeneratePipe[Out any] struct { ... }
func (g *GeneratePipe[Out]) Generate(ctx context.Context) (<-chan Out, error)
func (g *GeneratePipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error)

// ProcessPipe implements Processor and Pipe[In, Out]
type ProcessPipe[In, Out any] struct { ... }
func (p *ProcessPipe[In, Out]) Process(ctx context.Context, in In) ([]Out, error)
func (p *ProcessPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)

// BatchPipe implements BatchProcessor and Pipe[In, Out]
type BatchPipe[In, Out any] struct { ... }
func (b *BatchPipe[In, Out]) ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
func (b *BatchPipe[In, Out]) Pipe(ctx context.Context, in <-chan In) (<-chan Out, error)

// SinkPipe implements Sink and Pipe[In, struct{}]
type SinkPipe[In any] struct { ... }
func (s *SinkPipe[In]) Sink(ctx context.Context, in In) error
func (s *SinkPipe[In]) Pipe(ctx context.Context, in <-chan In) (<-chan struct{}, error)
```

### Constructors

```go
// Primary constructors - return concrete types
func NewGenerator[Out any](fn GenerateFunc[Out], cfg Config) *GeneratePipe[Out]
func NewProcessor[In, Out any](fn ProcessFunc[In, Out], cfg Config) *ProcessPipe[In, Out]
func NewBatchProcessor[In, Out any](fn BatchFunc[In, Out], cfg BatchConfig) *BatchPipe[In, Out]
func NewSink[In any](fn SinkFunc[In], cfg Config) *SinkPipe[In]

// Helper constructors - return ProcessPipe with adapted signatures
func NewFilter[T any](fn FilterFunc[T], cfg Config) *ProcessPipe[T, T]
func NewTransform[In, Out any](fn TransformFunc[In, Out], cfg Config) *ProcessPipe[In, Out]
```

### Function Type Aliases

```go
// Existing (unchanged)
type ProcessFunc[In, Out any] func(ctx context.Context, in In) ([]Out, error)

// New/renamed
type GenerateFunc[Out any] func(ctx context.Context) ([]Out, error)
type BatchFunc[In, Out any] func(ctx context.Context, batch []In) ([]Out, error)
type SinkFunc[In any] func(ctx context.Context, in In) error
type FilterFunc[T any] func(ctx context.Context, in T) (bool, error)
type TransformFunc[In, Out any] func(ctx context.Context, in In) (Out, error)
```

## Design Decisions

### 1. Semantic Methods Don't Apply Middleware

```go
proc := NewProcessor(fn, cfg)
proc.Use(middleware.Retry(...))

proc.Process(ctx, item)  // Calls fn directly, NO middleware
proc.Pipe(ctx, in)       // Applies middleware, full streaming path
```

**Rationale:**
- `Process()` is the raw function - useful for testing, single-item invocation
- `Pipe()` is the full pipeline with concurrency, middleware, error handling
- Avoids two code paths with different behavior
- Users who want middleware on single items can wrap manually

### 2. SinkPipe as Separate Type (not ProcessPipe alias)

**Before:** `NewSinkPipe() -> *ProcessPipe[In, struct{}]`
**After:** `NewSink() -> *SinkPipe[In]`

**Rationale:**
- `SinkPipe` can implement `Sink` interface with proper `Sink(In) error` signature
- Cleaner type in function signatures: `func NeedsSink(s Sink[T])`
- Output is always `struct{}` - no need for full ProcessPipe generics

### 3. Filter/Transform Stay as Helpers

`NewFilter` and `NewTransform` return `*ProcessPipe`, not separate types.

**Rationale:**
- They're just constrained versions of Process (1:1 or 1:0/1)
- No unique interface method needed
- Fewer types to maintain
- Users can still use type assertions if needed

### 4. GeneratePipe.Pipe() Accepts struct{} Channel

```go
func (g *GeneratePipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error)
```

**Rationale:**
- Enables `Apply(gen, proc)` composition
- Input is ignored internally (already the case)
- Explicit about the "trigger" semantics

## Tasks

### Task 1: Add Semantic Interfaces

**Files to Modify:**
- `pipe/pipe.go` - Add `Processor`, `BatchProcessor`, `Sink` interfaces
- `pipe/generator.go` - `Generator` interface already exists (keep)

**Changes:**
```go
// Add to pipe/pipe.go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

type BatchProcessor[In, Out any] interface {
    ProcessBatch(ctx context.Context, batch []In) ([]Out, error)
}

type Sink[In any] interface {
    Sink(ctx context.Context, in In) error
}
```

**Acceptance Criteria:**
- [ ] Interfaces defined in pipe/pipe.go
- [ ] Generator interface unchanged in pipe/generator.go

### Task 2: Add Process() Method to ProcessPipe

**Files to Modify:**
- `pipe/pipe.go` - Add `Process()` method to `ProcessPipe`

**Changes:**
```go
// Process invokes the handler for a single item without streaming.
// Does not apply middleware - use Pipe() for full pipeline behavior.
func (p *ProcessPipe[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
    return p.handle(ctx, in)
}
```

**Acceptance Criteria:**
- [ ] ProcessPipe has Process() method
- [ ] ProcessPipe implements Processor interface
- [ ] Process() calls handle directly (no middleware)

### Task 3: Add ProcessBatch() Method to BatchPipe

**Files to Modify:**
- `pipe/pipe.go` - Add `ProcessBatch()` method to `BatchPipe`

**Changes:**
```go
// ProcessBatch invokes the handler for a batch without streaming.
// Does not apply middleware - use Pipe() for full pipeline behavior.
func (p *BatchPipe[In, Out]) ProcessBatch(ctx context.Context, batch []In) ([]Out, error) {
    return p.handle(ctx, batch)
}
```

**Acceptance Criteria:**
- [ ] BatchPipe has ProcessBatch() method
- [ ] BatchPipe implements BatchProcessor interface
- [ ] ProcessBatch() calls handle directly (no middleware)

### Task 4: Add Pipe() Method to GeneratePipe

**Files to Modify:**
- `pipe/generator.go` - Add `Pipe()` method to implement `Pipe[struct{}, Out]`

**Changes:**
```go
// Pipe implements the Pipe interface for composition with Apply().
// The input channel is used as a trigger; values are ignored.
// Returns ErrAlreadyStarted if the generator has already been started.
func (g *GeneratePipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error) {
    return g.Generate(ctx)  // Input ignored, just use Generate()
}
```

**Design Note:** Should `Pipe()` drain the input channel or ignore it entirely?

Option A: Ignore input (current proposal)
```go
func (g *GeneratePipe[Out]) Pipe(ctx context.Context, in <-chan struct{}) (<-chan Out, error) {
    return g.Generate(ctx)
}
```

Option B: Use input as trigger (one output batch per input)
```go
// This would change Generator semantics significantly
```

**Recommendation:** Option A - Generator is inherently push-based, input is just for type compatibility.

**Acceptance Criteria:**
- [ ] GeneratePipe has Pipe() method
- [ ] GeneratePipe implements Pipe[struct{}, Out]
- [ ] Pipe() ignores input channel, calls Generate()

### Task 5: Create SinkPipe Type

**Files to Modify:**
- `pipe/pipe.go` - Add `SinkPipe` struct and methods

**Changes:**
```go
// SinkPipe is a Pipe that consumes items without producing meaningful output.
type SinkPipe[In any] struct {
    handle SinkFunc[In]
    cfg    Config
    mw     []middleware.Middleware[In, struct{}]

    mu      sync.Mutex
    started bool
}

// Sink invokes the handler for a single item without streaming.
// Does not apply middleware - use Pipe() for full pipeline behavior.
func (s *SinkPipe[In]) Sink(ctx context.Context, in In) error {
    return s.handle(ctx, in)
}

// Pipe begins consuming items from the input channel.
// Returns ErrAlreadyStarted if the sink has already been started.
func (s *SinkPipe[In]) Pipe(ctx context.Context, in <-chan In) (<-chan struct{}, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.started {
        return nil, ErrAlreadyStarted
    }
    s.started = true

    // Adapt SinkFunc to ProcessFunc for startProcessing
    fn := func(ctx context.Context, in In) ([]struct{}, error) {
        return nil, s.handle(ctx, in)
    }
    handle := applyMiddleware(fn, s.mw)
    return startProcessing(ctx, in, handle, s.cfg), nil
}

// Use adds middleware to the processing chain.
func (s *SinkPipe[In]) Use(mw ...middleware.Middleware[In, struct{}]) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.started {
        return ErrAlreadyStarted
    }
    s.mw = append(s.mw, mw...)
    return nil
}
```

**Acceptance Criteria:**
- [ ] SinkPipe struct defined
- [ ] SinkPipe implements Sink interface
- [ ] SinkPipe implements Pipe[In, struct{}]
- [ ] SinkPipe has Use() for middleware

### Task 6: Add Function Type Aliases

**Files to Modify:**
- `pipe/processing.go` - Add type aliases

**Changes:**
```go
// GenerateFunc is the function signature for generators.
type GenerateFunc[Out any] func(ctx context.Context) ([]Out, error)

// BatchFunc is the function signature for batch processors.
type BatchFunc[In, Out any] func(ctx context.Context, batch []In) ([]Out, error)

// SinkFunc is the function signature for sinks.
type SinkFunc[In any] func(ctx context.Context, in In) error

// FilterFunc is the function signature for filters.
type FilterFunc[T any] func(ctx context.Context, in T) (bool, error)

// TransformFunc is the function signature for transforms.
type TransformFunc[In, Out any] func(ctx context.Context, in In) (Out, error)
```

**Acceptance Criteria:**
- [ ] All function types defined
- [ ] Constructors updated to use named types

### Task 7: Rename Constructors

**Files to Modify:**
- `pipe/pipe.go` - Rename constructors
- `pipe/generator.go` - Keep NewGenerator (already good)

**Renames:**
| Before | After |
|--------|-------|
| `NewProcessPipe` | `NewProcessor` |
| `NewBatchPipe` | `NewBatchProcessor` |
| `NewFilterPipe` | `NewFilter` |
| `NewTransformPipe` | `NewTransform` |
| `NewSinkPipe` | `NewSink` |
| `NewGenerator` | `NewGenerator` (unchanged) |

**Acceptance Criteria:**
- [ ] All constructors renamed
- [ ] Old names removed (breaking change)

### Task 8: Update NewSink to Return *SinkPipe

**Files to Modify:**
- `pipe/pipe.go` - Change NewSink return type

**Before:**
```go
func NewSinkPipe[In any](handle func(context.Context, In) error, cfg Config) *ProcessPipe[In, struct{}]
```

**After:**
```go
func NewSink[In any](fn SinkFunc[In], cfg Config) *SinkPipe[In] {
    return &SinkPipe[In]{
        handle: fn,
        cfg:    cfg,
    }
}
```

**Acceptance Criteria:**
- [ ] NewSink returns *SinkPipe[In]
- [ ] SinkPipe implements both Sink and Pipe interfaces

### Task 9: Update Internal Tests

**Files to Modify:**
- `pipe/pipe_test.go` - Update constructor names
- `pipe/generator_test.go` - Add Pipe() method tests
- `pipe/processing_test.go` - Add Process()/ProcessBatch() tests

**New Test Cases:**
```go
func TestProcessPipe_Process(t *testing.T) {
    // Test direct invocation without channels
}

func TestBatchPipe_ProcessBatch(t *testing.T) {
    // Test direct invocation without channels
}

func TestGeneratePipe_Pipe(t *testing.T) {
    // Test Pipe interface implementation
}

func TestSinkPipe_Sink(t *testing.T) {
    // Test direct invocation
}

func TestSinkPipe_Pipe(t *testing.T) {
    // Test streaming behavior
}
```

**Acceptance Criteria:**
- [ ] All existing tests pass with new names
- [ ] New tests for semantic interface methods
- [ ] Tests verify middleware NOT applied to direct methods

### Task 10: Update message Package

**Files to Modify:**
- `message/pipes.go` - Update constructor calls
- `message/router.go` - Update constructor calls
- `message/cloudevents/publisher.go` - Update constructor calls
- `message/cloudevents/subscriber.go` - Update constructor calls

**Changes:**
```go
// Before
pipe.NewProcessPipe(fn, cfg)
pipe.NewSinkPipe(fn, cfg)
pipe.NewGenerator(fn, cfg)

// After
pipe.NewProcessor(fn, cfg)
pipe.NewSink(fn, cfg)
pipe.NewGenerator(fn, cfg)
```

**Acceptance Criteria:**
- [ ] All message package code compiles
- [ ] All message package tests pass

### Task 11: Update Documentation

**Files to Modify:**
- `pipe/doc.go` - Update package documentation
- `README.md` - Update examples
- `docs/adr/` - Create ADR for this change

**Acceptance Criteria:**
- [ ] Package docs reflect new API
- [ ] README examples use new constructors
- [ ] ADR documents rationale

## API Changes Summary

### Breaking Changes

| Before | After |
|--------|-------|
| `NewProcessPipe(fn, cfg)` | `NewProcessor(fn, cfg)` |
| `NewBatchPipe(fn, cfg)` | `NewBatchProcessor(fn, cfg)` |
| `NewFilterPipe(fn, cfg)` | `NewFilter(fn, cfg)` |
| `NewTransformPipe(fn, cfg)` | `NewTransform(fn, cfg)` |
| `NewSinkPipe(fn, cfg) -> *ProcessPipe` | `NewSink(fn, cfg) -> *SinkPipe` |

### New APIs

| API | Description |
|-----|-------------|
| `Processor[In, Out]` interface | Semantic interface with `Process()` |
| `BatchProcessor[In, Out]` interface | Semantic interface with `ProcessBatch()` |
| `Sink[In]` interface | Semantic interface with `Sink()` |
| `ProcessPipe.Process()` | Direct single-item invocation |
| `BatchPipe.ProcessBatch()` | Direct batch invocation |
| `GeneratePipe.Pipe()` | Pipe interface for composition |
| `SinkPipe` struct | Dedicated sink type |
| `SinkPipe.Sink()` | Direct single-item invocation |

### Unchanged

| API | Notes |
|-----|-------|
| `Pipe[In, Out]` interface | Core composition interface |
| `Generator[Out]` interface | Already exists |
| `NewGenerator()` | Name unchanged |
| `Apply()` | Works with all Pipe implementations |

## Implementation Order

```
Task 1 (interfaces) ──► Task 2 (Process) ──┐
                                           │
Task 3 (ProcessBatch) ─────────────────────┼──► Task 9 (tests)
                                           │
Task 4 (GeneratePipe.Pipe) ────────────────┤
                                           │
Task 5 (SinkPipe) ──► Task 8 (NewSink) ────┤
                                           │
Task 6 (type aliases) ─────────────────────┤
                                           │
Task 7 (rename constructors) ──────────────┴──► Task 10 (message pkg) ──► Task 11 (docs)
```

Tasks 1-6 can proceed in parallel.
Task 7 depends on all interface/struct changes.
Task 9 can start after Tasks 2-5.
Task 10 depends on Task 7.
Task 11 depends on Task 10.

## Open Questions

### 1. Should GeneratePipe.Pipe() consume or ignore input?

**Options:**
- A: Ignore input entirely (proposed)
- B: Use input as trigger (one Generate call per input value)
- C: Don't implement Pipe at all (Generator is special)

**Recommendation:** Option A - simplest, enables composition

### 2. Should Process()/Sink()/ProcessBatch() apply middleware?

**Options:**
- A: No middleware on direct methods (proposed)
- B: Apply middleware on all paths

**Recommendation:** Option A - keeps direct methods simple and predictable

### 3. What about FilterPipe and TransformPipe as separate types?

**Options:**
- A: Keep as helpers returning ProcessPipe (proposed)
- B: Create Filter and Transform interfaces and *Pipe types

**Recommendation:** Option A - they're just constrained processors, not semantically different

### 4. Should middleware types change?

Current: `middleware.Middleware[In, Out]` works with `ProcessFunc[In, Out]`

For SinkPipe: `middleware.Middleware[In, struct{}]` works but is awkward.

**Options:**
- A: Keep as-is (proposed)
- B: Add `SinkMiddleware[In]` type alias

**Recommendation:** Option A initially, can add alias later if needed

## Migration Guide

### Constructor Renames

```go
// Before
proc := pipe.NewProcessPipe(fn, cfg)
batch := pipe.NewBatchPipe(fn, cfg)
filter := pipe.NewFilterPipe(fn, cfg)
transform := pipe.NewTransformPipe(fn, cfg)
sink := pipe.NewSinkPipe(fn, cfg)

// After
proc := pipe.NewProcessor(fn, cfg)
batch := pipe.NewBatchProcessor(fn, cfg)
filter := pipe.NewFilter(fn, cfg)
transform := pipe.NewTransform(fn, cfg)
sink := pipe.NewSink(fn, cfg)
```

### Type Annotations

```go
// Before
var sink *pipe.ProcessPipe[MyType, struct{}]

// After
var sink *pipe.SinkPipe[MyType]
```

### Interface Usage

```go
// New capability: accept specific component types
func processItems(p pipe.Processor[int, string]) {
    // Can call p.Process(ctx, item) directly
    // Or p.Pipe(ctx, ch) for streaming
}

func consumeItems(s pipe.Sink[int]) {
    // Can call s.Sink(ctx, item) directly
}
```

## Acceptance Criteria

- [ ] All semantic interfaces defined (Processor, BatchProcessor, Sink)
- [ ] All structs implement their semantic interface
- [ ] GeneratePipe implements Pipe[struct{}, Out]
- [ ] SinkPipe is a separate type implementing Sink and Pipe
- [ ] All constructors renamed
- [ ] Direct methods (Process, ProcessBatch, Sink) don't apply middleware
- [ ] All pipe package tests pass
- [ ] All message package tests pass
- [ ] Build passes (`make build && make vet`)
- [ ] Documentation updated

## Trade-offs

### What We Gain
- **Type safety** - Accept `Processor[In,Out]` when you need a processor
- **Cleaner APIs** - `Sink[T]` vs `Pipe[T, struct{}]`
- **Direct invocation** - Test/use without channel setup
- **Self-documenting** - Interfaces describe capabilities

### What We Lose
- **API stability** - Breaking changes to constructor names
- **Simplicity** - More interfaces to understand
- **Consistency** - Direct methods behave differently than Pipe()

### Risks
- **Message package coupling** - Need to update consumers
- **Middleware confusion** - Users may expect Process() to apply middleware
- **GeneratePipe.Pipe() semantics** - Ignoring input may surprise users
