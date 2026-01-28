# Plan 0012: Engine Simplification

**Status:** Complete
**Related ADRs:** [0020](../../adr/0020-message-engine-architecture.md), [0023](../../adr/0023-engine-simplification.md)

## Overview

Simplify the message engine by removing complex shutdown tracking and multi-pool routing. The engine should be a simple orchestrator of merger, router, and distributor components. This addresses potential deadlock scenarios with loopback plugins and reduces overall complexity.

## Goals

1. Remove messageTracker and related drain detection logic
2. Remove multi-pool support (keep single pool configuration)
3. Remove auto-acking from Router (handler responsibility)
4. Remove loopback plugins entirely (deadlock risk with forced shutdown)
5. Simplify shutdown to natural cascading drain with consistent ShutdownTimeout

## Current Complexity

| Component | Lines | Purpose | Removal Reason |
|-----------|-------|---------|----------------|
| messageTracker | ~50 | Track in-flight for drain detection | Adds deadlock risk with loopback cycles |
| Multi-pool routing | ~100 | Per-handler concurrency | Internal Distributor+Merger in Router adds complexity |
| Auto-acking | ~40 | Ack/Nack in Router.process() | Mixed concerns, unclear ownership |
| Loopback special-casing | ~50 | Separate shutdown for cycles | Only needed due to tracker |
| Channel wrappers | ~80 | Track enter/exit for messages | Only needed for tracker |

## Tasks

### Task 1: Remove messageTracker

**Goal:** Eliminate drain detection and simplify shutdown to timeout-based behavior.

**Files to Modify:**
- `message/tracker.go` - Delete file
- `message/engine.go` - Remove tracker field, trackingMiddleware, AdjustInFlight

**Changes:**
- Remove `tracker *messageTracker` field
- Remove `trackingMiddleware()` function
- Remove `AdjustInFlight()` method
- Simplify shutdown goroutine to just wait for ShutdownTimeout

**Acceptance Criteria:**
- [x] tracker.go deleted
- [x] No tracker references in engine.go
- [x] Shutdown uses only ShutdownTimeout (no drain detection)

### Task 2: Remove Multi-Pool Support from Router

**Goal:** Simplify Router to always use single ProcessPipe.

**Files to Modify:**
- `message/router.go` - Remove multi-pool logic
- `message/engine.go` - Remove AddPoolWithConfig, AddHandlerToPool proxies

**Current Config (keep):**
```go
// RouterConfig - keep Pool field
type RouterConfig struct {
    BufferSize   int
    Pool         PoolConfig  // Single pool config
    ErrorHandler ErrorHandler
    Logger       Logger
}

// PoolConfig - keep as-is
type PoolConfig struct {
    Workers    int
    BufferSize int
}
```

**Remove:**
```go
// Remove from Router
pools        map[string]poolEntry
AddPoolWithConfig()
AddHandlerToPool()
pipeMultiPool()
makePoolMatcher()

// Remove from Engine
AddPoolWithConfig()
AddHandlerToPool()
```

**Simplify Router.Pipe():**
```go
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error) {
    // Always use single ProcessPipe - no multi-pool complexity
    cfg := pipe.Config{
        BufferSize:   r.cfg.Pool.BufferSize,
        Concurrency:  r.cfg.Pool.Workers,
        ErrorHandler: func(in any, err error) {
            msg := in.(*Message)
            r.errorHandler(msg, err)
        },
    }
    p := pipe.NewProcessPipe(r.process, cfg)
    return p.Pipe(ctx, in)
}
```

**Acceptance Criteria:**
- [x] Router uses single ProcessPipe always
- [x] No pools map, no AddPoolWithConfig, no AddHandlerToPool
- [x] Pool config preserved in PipeConfig and EngineConfig.RouterPool

### Task 3: Remove Auto-Acking from Router

**Goal:** Handler is responsible for acking/nacking messages.

**Files to Modify:**
- `message/router.go` - Remove Ack/Nack calls from process()

**Before:**
```go
func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
    entry, ok := r.handler(msg.Type())
    if !ok {
        msg.Nack(err)  // Remove
        return nil, ErrNoHandler
    }
    if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
        msg.Nack(err)  // Remove
        return nil, ErrHandlerRejected
    }
    outputs, err := entry.handler.Handle(ctx, msg)
    if err != nil {
        msg.Nack(err)  // Remove
        return nil, err
    }
    msg.Ack()  // Remove
    return outputs, nil
}
```

**After:**
```go
func (r *Router) process(ctx context.Context, msg *Message) ([]*Message, error) {
    entry, ok := r.handler(msg.Type())
    if !ok {
        return nil, ErrNoHandler
    }
    if entry.matcher != nil && !entry.matcher.Match(msg.Attributes) {
        return nil, ErrHandlerRejected
    }
    return entry.handler.Handle(ctx, msg)
}
```

**Add AutoAck Middleware:**
```go
// message/middleware.go - new file
package message

// AutoAck returns middleware that automatically acks on success, nacks on error.
// Use this to restore previous auto-acking behavior for handlers that don't
// need custom ack timing.
func AutoAck() Middleware {
    return func(next ProcessFunc) ProcessFunc {
        return func(ctx context.Context, msg *Message) ([]*Message, error) {
            results, err := next(ctx, msg)
            if err != nil {
                msg.Nack(err)
                return nil, err
            }
            msg.Ack()
            return results, nil
        }
    }
}
```

**Files to Create:**
- `message/middleware/autoack.go` - AutoAck middleware

**Acceptance Criteria:**
- [x] No Ack/Nack calls in Router.process()
- [x] AutoAck middleware created in message/middleware/autoack.go
- [x] Handler examples updated to show acking pattern

### Task 4: Remove Loopback Plugins Entirely

**Goal:** Remove loopback plugins due to deadlock risk under load.

**Rationale:** In production with high volume, the loopback buffer could fill up causing deadlocks
due to resource exhaustion when backpressure builds. The circular dependency
(merger → router → distributor → loopback → merger) means a slow consumer anywhere in the loop
blocks the entire pipeline. Additionally, forced shutdown drain logic compounds this by blocking
on channel closure in circular dependencies.

**Files to Delete:**
- `message/plugin/loopback.go` - Delete entire file
- `message/plugin/loopback_test.go` - Delete entire file

**Files to Modify:**
- `message/engine.go` - Remove loopback-specific methods and channels
- `message/engine_test.go` - Remove loopback tests
- `message/engine_graceful_shutdown_test.go` - Rewrite without loopback scenarios
- `message/engine_bench_test.go` - Rewrite without loopback benchmarks

**Remove from Engine:**
```go
loopbackShutdown chan struct{}
AddLoopbackInput()
AddLoopbackOutput()
wrapLoopbackOutputChannel()
```

**Migration:** Users needing loopback functionality should use an external message queue
(Redis, NATS, etc.) which provides natural buffering and avoids in-process circular dependencies.

**Acceptance Criteria:**
- [x] No loopbackShutdown channel
- [x] No AddLoopbackInput/AddLoopbackOutput methods
- [x] plugin/loopback.go deleted
- [x] plugin/loopback_test.go deleted
- [x] Tests updated to remove loopback scenarios

### Task 5: Simplify Channel Wrappers

**Goal:** Remove tracker-related channel wrapping.

**Files to Modify:**
- `message/engine.go` - Simplify or remove wrapper logic

**Remove:**
```go
wrapperWg sync.WaitGroup
wrapChannel[T]()
wrapInputChannel()
wrapRawInputChannel()
wrapOutputChannel()
wrapLoopbackOutputChannel()
```

**Simplify Input/Output Methods:**
```go
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) error {
    e.cfg.Logger.Info("Adding input", "component", "engine", "input", name)
    filtered := e.applyTypedInputMatcher(name, ch, matcher)
    _, err := e.merger.AddInput(filtered)
    return err
}

func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error) {
    e.cfg.Logger.Info("Adding output", "component", "engine", "output", name)
    return e.distributor.AddOutput(func(msg *Message) bool {
        return matcher == nil || matcher.Match(msg.Attributes)
    })
}
```

**Acceptance Criteria:**
- [x] No wrapperWg field
- [x] No wrapChannel helper
- [x] Input/Output methods directly use merger/distributor

### Task 6: Simplify Shutdown Orchestration

**Goal:** Shutdown is just context cancellation with timeout on merger.

**Before (complex):**
```go
go func() {
    <-ctx.Done()
    e.tracker.close()

    if e.cfg.ShutdownTimeout > 0 {
        select {
        case <-e.tracker.drained():
        case <-time.After(e.cfg.ShutdownTimeout):
        }
    } else {
        <-e.tracker.drained()
    }

    close(e.loopbackShutdown)
    close(e.shutdown)
}()
```

**After (simple):**
```go
// Shutdown happens via:
// 1. User cancels context
// 2. Merger's ShutdownTimeout forces close if inputs don't close
// 3. Cascading drain: merger → router → distributor → outputs
// No additional orchestration needed
```

**Acceptance Criteria:**
- [x] Shutdown goroutine simplified (signals marshal/unmarshal pipes only)
- [x] Merger handles timeout via its own ShutdownTimeout
- [x] Cascading drain: merger → router → distributor → outputs

## Simplified Architecture

```
Engine (after simplification):
  - cfg: EngineConfig
  - merger: *pipe.Merger[*Message]
  - router: *Router
  - distributor: *pipe.Distributor[*Message]
  - mu: sync.Mutex
  - started: bool

Flow:
  inputs → merger → router → distributor → outputs

Shutdown (cascading drain):
  context.Cancel() → merger (with ShutdownTimeout) → router → distributor → outputs
```

## API Changes

### Removed Methods
```go
// Engine
AddPoolWithConfig(name string, cfg PoolConfig) error
AddHandlerToPool(name string, matcher Matcher, h Handler, pool string) error
AddLoopbackInput(name string, matcher Matcher, ch <-chan *Message) error
AddLoopbackOutput(name string, matcher Matcher) (<-chan *Message, error)
AdjustInFlight(delta int)
```

### Signatures (unchanged)
```go
// AddInput returns done channel for input consumption tracking
AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error)

// AddRawInput returns done channel for input consumption tracking
AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error)
```

### Config
```go
type EngineConfig struct {
    Marshaler       Marshaler
    BufferSize      int
    RouterPool      PoolConfig  // Single pool config
    ErrorHandler    ErrorHandler
    Logger          Logger
    ShutdownTimeout time.Duration
}

type PipeConfig struct {
    Pool            PoolConfig
    ShutdownTimeout time.Duration
    Logger          Logger
    ErrorHandler    ErrorHandler
}
```

## Implementation Order

```
Task 1 (tracker) ─┬─► Task 4 (loopback) ─┬─► Task 6 (shutdown)
                  │                       │
Task 2 (multi-pool)                       │
                                          │
Task 3 (auto-ack)                         │
                                          │
Task 5 (wrappers) ────────────────────────┘
```

Tasks 1, 2, 3 can proceed in parallel.
Task 4 depends on Task 1 (loopback special-casing exists due to tracker).
Task 5 depends on Task 1 (wrappers exist for tracking).
Task 6 depends on Tasks 4 and 5.

## Trade-offs

### What We Lose
- **Per-handler concurrency control** - users need multiple engines or in-handler concurrency
- **Automatic drain detection** - shutdown is timeout-based only
- **Auto-acking convenience** - handlers must ack explicitly
- **In-process loopbacks** - use external message queue for re-routing

### What We Gain
- **~320 lines less code**
- **No deadlock risk** from tracker coordination or loopback resource exhaustion
- **Simpler mental model** - just merger → router → distributor
- **Clearer shutdown** - close inputs or wait for timeout
- **Handler autonomy** - full control over acking timing

## Migration Guide

### Multi-Pool Users
```go
// Before
e.AddPoolWithConfig("fast", PoolConfig{Workers: 10})
e.AddHandlerToPool("handler", nil, h, "fast")

// After - use higher concurrency in single pool
e := NewEngine(EngineConfig{
    RouterPool: PoolConfig{Workers: 10},
})
e.AddHandler("handler", nil, h)
```

### Auto-Ack Users
```go
// Option 1: Use AutoAck middleware (restores previous behavior)
e.Use(middleware.AutoAck())

// Option 2: Handler acks explicitly (recommended for fine-grained control)
func (h *MyHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    result, err := process(msg)
    if err != nil {
        msg.Nack(err)
        return nil, err
    }
    msg.Ack()
    return result, nil
}
```

### Loopback Users
```go
// Before
e.AddPlugin(plugin.Loopback("batch", matcher))

// After - use external message queue
// Loopbacks are removed due to deadlock risk during forced shutdown.
// Use an external queue (Redis, NATS, etc.) for message re-routing:
//
//   output, _ := e.AddOutput("batch-out", matcher)
//   go func() {
//       for msg := range output {
//           externalQueue.Publish(msg)  // Non-blocking external publish
//       }
//   }()
//
//   // Separate consumer reads from external queue and feeds back to engine
//   e.AddInput("batch-in", nil, externalQueueConsumer)
```

## Acceptance Criteria

- [x] All tasks completed
- [x] tracker.go deleted
- [x] No multi-pool references
- [x] No auto-acking in Router (AutoAck middleware available)
- [x] Loopback plugins removed entirely
- [x] Tests updated and passing
- [x] ShutdownTimeout propagated to all components
- [ ] CHANGELOG updated (pending release)
- [ ] Migration guide in release notes (pending release)

## Implementation Notes

Tasks 5 and 6 were simplified from the original plan:
- Channel wrappers retained (simplified) for shutdown coordination
- Shutdown goroutine retained (simplified) to signal marshal/unmarshal pipes
- These are minimal and serve clear purposes: timeout coordination for inputs, shutdown signaling for outputs

### Shutdown Timeout Semantics

ShutdownTimeout is now consistent across all components (pipe and message packages):
- `<= 0`: Forces immediate shutdown (no grace period)
- `> 0`: Waits up to this duration for natural completion, then forces shutdown

On forced shutdown, workers exit immediately without blocking on channel drains.
This prevents deadlocks that occurred when drain logic (`for v := range ch`) blocked
waiting for channels to close.

### Loopback Removal

Task 4 evolved from "simplify loopbacks" to "remove loopbacks entirely" because:
1. **Production deadlock risk**: Under high volume, the loopback buffer fills up causing
   deadlocks due to resource exhaustion when backpressure builds
2. **Circular dependency**: merger → router → distributor → loopback → merger means a slow
   consumer anywhere in the loop blocks the entire pipeline
3. **Shutdown complications**: Forced shutdown drain logic compounds this by blocking on
   channel closure in circular dependencies

Users needing loopback functionality should use external message queues which provide
natural buffering, backpressure handling, and avoid in-process circular dependencies.
