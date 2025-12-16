# Layer 0: Foundation Cleanup

**Status:** Proposed
**Priority:** PREREQUISITE - Must be done first
**Related ADRs:** 0026, 0027, 0028
**Related Features:** 16

## Overview

Before implementing CloudEvents standardization, we must simplify core abstractions. This layer cleans up technical debt and establishes patterns for subsequent layers.

## Goals

1. Replace generic functional options with `ProcessorConfig` struct
2. Consolidate middleware into dedicated package
3. Replace `Generator` with `Subscriber` hierarchy
4. Add configurable fan-out with `BroadcastConfig`
5. Add `RoutingFanOut` for destination-based routing

## Sub-Tasks

### Task 0.1: ProcessorConfig Struct

**Goal:** Replace `Option[In, Out]` with non-generic `ProcessorConfig`

**Current:**
```go
pipe := NewProcessPipe(
    handler,
    WithConcurrency[Order, ShippingCommand](4),
    WithBuffer[Order, ShippingCommand](100),
    WithTimeout[Order, ShippingCommand](5*time.Second),
)
```

**Target:**
```go
pipe := NewProcessPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Buffer:      100,
    Timeout:     5 * time.Second,
})
```

**Files to Create/Modify:**
- `processor_config.go` (new) - ProcessorConfig struct
- `processor.go` (modify) - Accept config, deprecate options
- `pipe.go` (modify) - Update pipe constructors
- `deprecated.go` (new) - Backward-compat wrappers

**Acceptance Criteria:**
- [ ] `ProcessorConfig` struct defined with all settings
- [ ] `StartProcessor` accepts config instead of variadic options
- [ ] All pipe constructors updated
- [ ] Deprecated wrappers for backward compatibility
- [ ] Tests pass with new API

---

### Task 0.2: Drop Path Refactoring

**Goal:** Make drop handling explicit with unified handler and config fallback

**Analysis:** See [cancel-path-refactoring/](cancel-path-refactoring/) for detailed evaluation of 6 options.

**Decision:** Option 5 - Hybrid (Explicit Drop + Config Default)

#### Key Design Decisions

1. **Keep the cancel goroutine** - It's a resilience mechanism preventing deadlocks
2. **Single unified handler** - Handles both errors and cancellations
3. **Error type distinguishes cause** - `ErrCancel` vs `ErrFailure`
4. **Explicit drop parameter** - In pipe constructors (nil = use config fallback)
5. **Non-generic config** - `OnDrop func(any, error)` enables sharing across types

#### Naming Options (Decision Pending)

| Current | Option 1 | Option 2 | Option 3 | Notes |
|---------|----------|----------|----------|-------|
| `Cancel` | `Drop` | `Reject` | `Discard` | Method on Processor |
| `CancelFunc` | `DropFunc` | `RejectFunc` | `DiscardFunc` | Type alias |
| `OnCancel` | `OnDrop` | `OnReject` | `OnDiscard` | Config field |
| `ErrCancel` | `ErrCanceled` | - | - | Keep (matches context) |
| `ErrFailure` | `ErrFailed` | `ErrProcessFailed` | - | Sentinel error |

**Preferred:** `Drop` / `OnDrop` - accurately describes what happens (item dropped from pipeline)

#### Current vs Target

**Current:** (processor.go)
```go
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)  // Called for both errors and cancellations
}

// Pipes pass nil, masking the explicit path
proc := NewProcessor(handle, nil)
```

**Target:**
```go
// Processor interface (consider renaming Cancel to Drop)
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Drop(In, error)  // Renamed from Cancel
}

// Sentinel errors distinguish cause
var (
    ErrCanceled = errors.New("gopipe: canceled")
    ErrFailed   = errors.New("gopipe: processing failed")
)

// Non-generic config with unified handler
type ProcessorConfig struct {
    Concurrency int
    Buffer      int
    OnDrop      func(input any, err error)  // Fallback handler
}

// Pipe constructors with explicit drop parameter
func NewProcessPipe[In, Out any](
    process func(context.Context, In) ([]Out, error),
    drop func(In, error),  // Explicit, type-safe (nil = use config.OnDrop)
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out]
```

#### Example Implementation

```go
// Shared config - reusable across processors of different types
sharedConfig := ProcessorConfig{
    Concurrency: 4,
    OnDrop: func(input any, err error) {
        if errors.Is(err, ErrCanceled) {
            slog.Warn("item canceled", "type", fmt.Sprintf("%T", input))
        } else {
            slog.Error("item failed", "type", fmt.Sprintf("%T", input))
        }
    },
}

// Pipe 1: Order -> ShippingCommand (uses config fallback)
orderPipe := NewProcessPipe(orderHandler, nil, sharedConfig)

// Pipe 2: Payment -> Notification (same config reused!)
paymentPipe := NewProcessPipe(paymentHandler, nil, sharedConfig)

// Pipe 3: Critical orders (explicit type-safe handler)
criticalPipe := NewProcessPipe(
    orderHandler,
    func(order Order, err error) {
        // Type-safe: can access order.ID directly
        alerting.Send("VIP order dropped: " + order.ID)
    },
    sharedConfig,
)
```

**Runnable example:** [cancel-path-refactoring/main.go](cancel-path-refactoring/main.go)

#### Files to Modify

- `errors.go` - Add `ErrCanceled`, `ErrFailed` (or keep existing names)
- `processor.go` - Rename `Cancel` to `Drop`, update goroutine logic
- `processor_config.go` - Add `OnDrop` field
- `pipe.go` - Add explicit `drop` parameter to constructors
- `option.go` - Deprecate `WithCancel`, add `WithDrop`

#### Acceptance Criteria

- [ ] Cancel goroutine preserved (resilience mechanism)
- [ ] Single `OnDrop` handler in ProcessorConfig
- [ ] Explicit drop parameter in pipe constructors
- [ ] Error types distinguish cancel vs failure
- [ ] Runnable example demonstrates shared config
- [ ] Tests cover both error paths

---

### Task 0.3: Middleware Package Consolidation

**Goal:** Move all middleware to `middleware/` package with explicit composition

**Analysis:** See [middleware-refactoring/](middleware-refactoring/) for detailed evaluation

**Decision:** Option A - Full Migration

#### Key Design Decisions

1. **Move all middleware** from root to `middleware/` package
2. **Explicit composition** via `Chain()` function
3. **No implicit behavior** - logging becomes opt-in (breaking change)
4. **Config struct pattern** for all middleware
5. **Drop handler middleware** wraps `Drop()` method

#### Package Structure

```
middleware/
├── doc.go              # Package documentation
├── middleware.go       # Core types, Chain(), Apply()
├── timeout.go          # Context timeout
├── retry.go            # Retry with backoff
├── recover.go          # Panic recovery
├── metrics.go          # Metrics collection (interface-based)
├── logging.go          # Structured logging (slog)
├── drop.go             # Drop/cancel handler
├── message/            # Message-specific
│   ├── correlation.go  # Correlation ID propagation
│   ├── validation.go   # CloudEvents validation (future)
│   └── tracing.go      # Distributed tracing (future)
└── internal/
    └── backoff.go      # Backoff implementations
```

#### Core Types

```go
// Middleware wraps a Processor with additional behavior.
type Middleware[In, Out any] = gopipe.MiddlewareFunc[In, Out]

// Chain combines multiple middleware into one.
// Execution order: first middleware is outermost.
func Chain[In, Out any](mw ...Middleware[In, Out]) Middleware[In, Out]
```

#### Migration Table

| Current | New | Notes |
|---------|-----|-------|
| `gopipe.WithRetryConfig` | `middleware.Retry` | Config struct |
| `gopipe.WithRecover` | `middleware.Recover` | Config struct |
| `gopipe.WithTimeout` | `middleware.Timeout` | Config struct |
| `gopipe.WithMetricsCollector` | `middleware.Metrics` | Interface-based |
| `gopipe.WithLogConfig` | `middleware.Logging` | Opt-in |
| `gopipe.WithMetadataProvider` | `middleware.Metadata` | Config struct |

#### Example Usage

```go
// Reusable middleware chain
productionMiddleware := middleware.Chain(
    middleware.Recover[Order, Cmd](middleware.RecoverConfig{
        OnPanic: func(input any, val any, stack string) { ... },
    }),
    middleware.Logging[Order, Cmd](middleware.LoggingConfig{
        Logger:     slog.Default(),
        OnDrop:     true,
    }),
    middleware.Metrics[Order, Cmd](middleware.MetricsConfig{
        Recorder: prometheusRecorder,
    }),
    middleware.Timeout[Order, Cmd](middleware.TimeoutConfig{
        Duration: 5 * time.Second,
    }),
    middleware.Retry[Order, Cmd](middleware.RetryConfig{
        MaxAttempts: 3,
        Backoff:     middleware.ExponentialBackoff(100*time.Millisecond, 2, 5*time.Second, 0.2),
    }),
)

// Apply to pipe
pipe := gopipe.NewProcessPipe(handler, nil, config, productionMiddleware)
```

**Runnable example:** [middleware-refactoring/main.go](middleware-refactoring/main.go)

#### Breaking Changes

1. **Implicit logging removed** - Must explicitly add `middleware.Logging`
2. **Option functions deprecated** - Use middleware directly
3. **MetricsCollector → MetricsRecorder** - Interface-based

#### Acceptance Criteria

- [ ] All middleware moved to `middleware/` package
- [ ] `Chain()` function for combining middleware
- [ ] Deprecated wrappers in `gopipe/deprecated.go`
- [ ] Logging middleware uses `log/slog` directly
- [ ] Drop handler middleware wraps `Drop()` method
- [ ] Runnable example demonstrates shared message middleware chain
- [ ] Tests cover all middleware

---

### Task 0.4: BroadcastConfig

**Goal:** Add configurable broadcast with slow-receiver handling

**Current:** (channel/broadcast.go)
```go
func Broadcast[T any](in <-chan T, n int) []<-chan T
```

**Target:**
```go
type BroadcastConfig struct {
    Buffer         int
    OnSlowReceiver func(index int, val any, elapsed time.Duration)
    SlowThreshold  time.Duration
}

func BroadcastWithConfig[T any](in <-chan T, n int, config BroadcastConfig) []<-chan T
```

**Files to Modify:**
- `channel/broadcast.go` - Add BroadcastWithConfig
- `channel/broadcast_test.go` - Add tests

**Backward Compatibility:** Keep existing `Broadcast()` function unchanged

---

### Task 0.5: RoutingFanOut

**Goal:** Add destination-based fan-out (routes to ONE output)

**Implementation:**
```go
type RoutingFanOut[T any] struct {
    router   RoutingFunc[T]
    outputs  map[string]chan<- T
    config   RoutingFanOutConfig
}

type RoutingFunc[T any] func(msg T) string

type RoutingFanOutConfig struct {
    DefaultOutput   string
    OnNoMatch       func(msg any)
    OnSlowConsumer  func(dest string, msg any, elapsed time.Duration)
    BufferPerOutput int
}
```

**Use Case:**
```go
fanout := NewRoutingFanOut(
    func(msg *Message) string {
        dest := msg.Destination()
        if idx := strings.Index(dest, "://"); idx != -1 {
            return dest[:idx+3]  // "kafka://"
        }
        return "default"
    },
    RoutingFanOutConfig{BufferPerOutput: 100},
)

fanout.AddOutput("kafka://", kafkaChan)
fanout.AddOutput("gopipe://", loopChan)
fanout.Start(ctx, routerOutput)
```

**Files to Create:**
- `channel/routing_fanout.go` (new)
- `channel/routing_fanout_test.go` (new)

---

### Task 0.6: Subscriber Interface

**Goal:** Replace `Generator[Out]` with `Subscriber[Out]` hierarchy

**Current:**
```go
type Generator[Out any] interface {
    Generate(ctx context.Context) <-chan Out
}
```

**Target:**
```go
type Subscriber[Out any] interface {
    Subscribe(ctx context.Context) <-chan Out
}

// Specialized types
func NewTickerSubscriber[Out any](interval time.Duration, generate func(ctx, time.Time) ([]Out, error), config SubscriberConfig) Subscriber[Out]
func NewPollingSubscriber[Out any](interval time.Duration, poll func(ctx) ([]Out, error), config SubscriberConfig) Subscriber[Out]
func NewFuncSubscriber[Out any](generate func(ctx) ([]Out, error), config SubscriberConfig) Subscriber[Out]

// For external brokers
type BrokerSubscriber interface {
    Subscriber[*Message]
    Acknowledge(ctx context.Context, msg *Message) error
    Seek(ctx context.Context, offset int64) error
}
```

**Files to Create/Modify:**
- `subscriber.go` (new) - Subscriber interface and factories
- `subscriber_config.go` (new) - SubscriberConfig struct
- `generator.go` (modify) - Deprecate, delegate to Subscriber
- `message/subscriber.go` (modify) - Update to use new interface

---

### Task 0.7: Package Restructuring

**Goal:** Restructure gopipe to focus on messaging, move generic primitives to subpackage

**Analysis:** See [package-restructuring.md](package-restructuring.md) for detailed plan

#### Key Design Decisions

1. **Main package focuses on messaging** - Router, Handler, Publisher, Subscriber
2. **Generic pipes move to `pipe/`** - Processor[In, Out], Pipe[In, Out]
3. **Message entity stays in `message/`** - Message, Attributes, Acking only
4. **Middleware is message-specific** - Stays in `middleware/`
5. **Subpackages promoted** - broker/, cloudevents/, cqrs/ become top-level

#### New Package Hierarchy

```
gopipe/                 # Message pipeline framework
├── message/            # Message entity only
├── pipe/               # Generic processor/pipe primitives
├── middleware/         # Message middleware
├── broker/             # Broker implementations
├── cloudevents/        # CloudEvents support
├── cqrs/               # CQRS patterns
└── channel/            # Channel utilities
```

#### Migration Summary

| From | To |
|------|-----|
| `message.Router` | `gopipe.Router` |
| `message.Handler` | `gopipe.Handler` |
| `message.Publisher` | `gopipe.Publisher` |
| `message.Subscriber` | `gopipe.Subscriber` |
| `gopipe.Processor[I,O]` | `pipe.Processor[I,O]` |
| `gopipe.NewProcessPipe` | `pipe.NewProcessPipe` |
| `message/broker.*` | `broker.*` |

#### Acceptance Criteria

- [ ] `pipe/` package contains all generic processor/pipe types
- [ ] Main `gopipe/` package exports Router, Handler, Publisher, Subscriber
- [ ] `message/` package contains only Message entity and related types
- [ ] `middleware/` package contains all middleware implementations
- [ ] `broker/`, `cloudevents/`, `cqrs/` are top-level packages
- [ ] Deprecated wrappers provide backward compatibility
- [ ] All tests pass
- [ ] Examples updated to new import paths

---

## Implementation Order

```
1. ProcessorConfig ──────────────────────┐
                                         │
2. Drop Path Refactoring ────────────────┼──► 4. BroadcastConfig
                                         │
3. Middleware Package ───────────────────┤
                                         │
                                         └──► 5. RoutingFanOut

6. Subscriber Interface ─────────────────────► Depends on 1, 2

7. Package Restructuring ────────────────────► Depends on 1, 2, 3, 6
```

**Recommended PR Sequence:**
1. **PR 1:** ProcessorConfig + Drop Path Refactoring
2. **PR 2:** Middleware Package Consolidation
3. **PR 3:** BroadcastConfig + RoutingFanOut
4. **PR 4:** Subscriber Interface
5. **PR 5:** Package Restructuring (major refactoring)

## Validation Checklist

Before marking Layer 0 complete:

- [ ] All tests pass with new APIs
- [ ] Deprecated functions have runtime warnings
- [ ] No generic type parameters in config structs
- [ ] Middleware package is self-contained
- [ ] RoutingFanOut tested with message types
- [ ] Subscriber hierarchy documented
- [ ] Package restructuring complete with backward compat
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0026: Pipe Simplification](../adr/0026-pipe-processor-simplification.md)
- [ADR 0027: Fan-Out Pattern](../adr/0027-fan-out-pattern.md)
- [ADR 0028: Subscriber Patterns](../adr/0028-generator-source-patterns.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
- [Package Restructuring](package-restructuring.md)
