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

### Task 0.2: Simplified Cancel Path

**Goal:** Remove dedicated cancel goroutine from processor

**Current:** (processor.go:151-159)
```go
wgCancel := sync.WaitGroup{}
wgCancel.Add(1)
go func() {
    <-ctx.Done()
    for val := range in {
        proc.Cancel(val, newErrCancel(ctx.Err()))
    }
    wgCancel.Done()
}()
```

**Target:**
```go
// Workers just return on context cancellation
select {
case <-ctx.Done():
    return  // No drain
case val, ok := <-in:
    // ...
}

// Error handling via config callback
if config.OnError != nil {
    config.OnError(val, err)
}
```

**Files to Modify:**
- `processor.go` - Remove cancel goroutine, add OnError callback

**Trade-offs:**
- **Lost:** Draining in-flight items on cancel
- **Gained:** Simpler model, one less goroutine per processor
- **Mitigation:** Use message acknowledgments for reliability

---

### Task 0.3: Middleware Package Consolidation

**Goal:** Move all middleware to `middleware/` package

**Structure:**
```
middleware/
├── middleware.go       # Core types, Chain()
├── timeout.go          # Context timeout
├── retry.go            # Retry (moved from root)
├── metrics.go          # PrometheusCollector
├── logging.go          # slog/zap adapters
├── recover.go          # Panic recovery
├── message/            # Message-specific
│   ├── correlation.go
│   ├── validation.go
│   └── routing.go
└── doc.go
```

**Migration Table:**
| Old Location | New Location |
|--------------|--------------|
| `gopipe.WithRetryConfig` | `middleware.WithRetry` |
| `gopipe.RetryConfig` | `middleware.RetryConfig` |
| `gopipe.useMetrics` | `middleware.WithMetrics` |
| `gopipe.useRecover` | `middleware.WithRecover` |
| `gopipe.useContext` | `middleware.WithTimeout` |

**Acceptance Criteria:**
- [ ] All middleware in `middleware/` package
- [ ] Deprecated wrappers in `gopipe/deprecated.go`
- [ ] `middleware.Chain()` for combining middleware
- [ ] Logger adapters for slog and zap
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

## Implementation Order

```
1. ProcessorConfig ──────────────────────┐
                                         │
2. Simplified Cancel ────────────────────┼──► 4. BroadcastConfig
                                         │
3. Middleware Package ───────────────────┤
                                         │
                                         └──► 5. RoutingFanOut

6. Subscriber Interface ─────────────────────► Depends on 1, 2
```

**Recommended PR Sequence:**
1. **PR 1:** ProcessorConfig + Simplified Cancel
2. **PR 2:** Middleware Package Consolidation
3. **PR 3:** BroadcastConfig + RoutingFanOut
4. **PR 4:** Subscriber Interface

## Validation Checklist

Before marking Layer 0 complete:

- [ ] All tests pass with new APIs
- [ ] Deprecated functions have runtime warnings
- [ ] No generic type parameters in config structs
- [ ] Middleware package is self-contained
- [ ] RoutingFanOut tested with message types
- [ ] Subscriber hierarchy documented
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0026: Pipe Simplification](../adr/0026-pipe-processor-simplification.md)
- [ADR 0027: Fan-Out Pattern](../adr/0027-fan-out-pattern.md)
- [ADR 0028: Subscriber Patterns](../adr/0028-generator-source-patterns.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
