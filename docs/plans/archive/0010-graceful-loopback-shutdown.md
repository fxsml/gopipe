# Plan 0010: Graceful Loopback Shutdown

**Status:** Complete
**Issue:** #81
**Related ADRs:** None (new feature)

## Overview

Enable graceful shutdown for pipelines with loopbacks without heuristics. Uses reference counting to detect when the pipeline is drained, then closes loopback outputs to break the cycle.

## Goals

1. Deterministic shutdown without idle-time heuristics
2. Works for all loopback topologies (simple, batch, group, chained)
3. No API changes for plugin **users** (internal plugin code changes, but `plugin.Loopback(...)` calls unchanged)
4. Components remain decoupled - Engine owns all tracking logic
5. Minimal runtime overhead

## Problem

Loopback creates a circular dependency that deadlocks on shutdown:

```
                   ┌─────────────────────────────────────┐
                   │                                     │
                   ↓                                     │
External Input → Merger → Router → Distributor → Loopback Output
                                          │
                                          └→ External Output
```

**Deadlock sequence:**
1. Context cancelled
2. Merger waits for all inputs to close
3. Loopback input waits for Distributor output to close
4. Distributor waits for Router to close
5. Router waits for Merger to close
6. **DEADLOCK**

## Solution: Reference Counting with Engine Wrapping

Track messages through the pipeline via channel wrappers. Components remain decoupled - only Engine knows about tracking.

### Architecture

```
                      ┌──────────────────────────────────────────────────────────┐
                      │                         Engine                            │
                      │   ┌────────────────────────────────────────────────────┐  │
                      │   │                 MessageTracker                      │  │
                      │   │              (Enter / Exit / Drained)               │  │
                      │   └────────────────────────────────────────────────────┘  │
                      │          ▲                ▲                ▲              │
                      │          │                │                │              │
External ─► AddInput ─┼─► [wrap Enter] ─► Merger ─► Router ─► Distributor ───────┼─► AddOutput ─► [wrap Exit] ─► External
                      │                              ▲                │           │
                      │                              │                ▼           │
Loopback ◄─ AddLoopbackInput ◄───────────────────────┴──── AddLoopbackOutput ◄───┘
                      │          (no wrap)                      (no wrap)         │
                      └──────────────────────────────────────────────────────────┘
```

### Counting Rules

| Event | Action | Location |
|-------|--------|----------|
| Message enters from external input | `Enter()` | Engine input wrapper |
| Message enters from external raw input | `Enter()` | Engine raw input wrapper |
| Message exits to external output | `Exit()` | Engine output wrapper |
| Message exits to external raw output | `Exit()` | Engine raw output wrapper |
| Handler drops message (returns 0) | `Exit()` | Engine handler wrapper |
| Handler multiplies message (returns N>1) | `Enter()` × (N-1) | Engine handler wrapper |
| Handler returns error | `Exit()` | Engine handler wrapper |
| Message goes to loopback | Nothing | Not wrapped |
| Message returns from loopback | Nothing | Not wrapped |

### Shutdown Sequence

```
t0    ctx.Cancel()
t1    Engine calls tracker.Close() to signal no more external inputs
t2    Wait for tracker.Drained() OR ShutdownTimeout
t3    Close loopback outputs (breaks cycle)
t4    Loopback inputs see closed channel
t5    Merger completes
t6    Pipeline drains naturally
t7    Wrapper goroutines complete (tracked via WaitGroup)
t8    Engine done channel closes
```

## API Design

### Engine (public)

```go
// External - tracked
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error)
func (e *Engine) AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error)
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error)
func (e *Engine) AddRawOutput(name string, matcher Matcher) (<-chan *RawMessage, error)

// Loopback - not tracked, marked for shutdown coordination
func (e *Engine) AddLoopbackInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error)
func (e *Engine) AddLoopbackOutput(name string, matcher Matcher) (<-chan *Message, error)
```

### Distributor (internal)

```go
// Existing
func (d *Distributor[T]) AddOutput(matcher func(T) bool) (<-chan T, error)

// New
func (d *Distributor[T]) AddLoopbackOutput(matcher func(T) bool) (<-chan T, error)
func (d *Distributor[T]) CloseLoopbackOutputs()  // Idempotent, safe to call multiple times
```

### MessageTracker (new, internal)

```go
// MessageTracker tracks in-flight messages for graceful shutdown.
// Enter() increments count, Exit() decrements. Drained() closes when
// count reaches zero AND Close() has been called.
type MessageTracker struct {
    inFlight atomic.Int64
    closed   atomic.Bool
    drained  chan struct{}
    once     sync.Once
}

func NewMessageTracker() *MessageTracker
func (t *MessageTracker) Enter()                    // Increment in-flight count
func (t *MessageTracker) Exit()                     // Decrement; signal drained if zero and closed
func (t *MessageTracker) Close()                    // Signal no more Enter() calls expected
func (t *MessageTracker) Drained() <-chan struct{}  // Closed when count=0 and Close() called
func (t *MessageTracker) InFlight() int64           // Current count (for debugging/metrics)
```

**Key behavior:**
- `Drained()` only closes when BOTH conditions met: `Close()` called AND `inFlight == 0`
- This handles the edge case where no messages ever enter (count stays 0)

## Analysis

### Goals Verification

| Goal | Status | Notes |
|------|--------|-------|
| Deterministic shutdown without heuristics | ✅ | Reference counting is deterministic |
| Works for all loopback topologies | ✅ | Tests cover simple, batch, group, chained |
| No API changes for plugin users | ✅ | `plugin.Loopback(...)` signature unchanged |
| Components remain decoupled | ✅ | Engine owns all tracking via wrappers |
| Minimal runtime overhead | ✅ | Benchmarks validate <5% overhead target |

### Edge Cases Coverage

| Edge Case | Covered | Test |
|-----------|---------|------|
| Handler returns 0 messages | ✅ | `TestTrackedHandler_Drop` |
| Handler returns N > 1 messages | ✅ | `TestTrackedHandler_Multiply` |
| Handler returns error | ✅ | `TestTrackedHandler_Error` |
| No external inputs (loopback only) | ✅ | `TestEngine_GracefulShutdown_NoExternalInputs` |
| No loopbacks (baseline) | ✅ | `TestEngine_ShutdownWithoutLoopback` |
| No messages sent before shutdown | ✅ | `TestEngine_GracefulShutdown_NoMessages` |
| Multiple loopbacks | ✅ | `TestEngine_GracefulShutdown_MultipleLoopbacks` |
| Chained loopbacks | ✅ | `TestEngine_GracefulShutdown_ChainedLoopbacks` |
| Shutdown before Start() | ✅ | `TestEngine_ShutdownBeforeStart` |
| AddLoopbackOutput after Start() | ✅ | `TestDistributor_AddLoopbackOutput_AfterStart` |
| CloseLoopbackOutputs called twice | ✅ | `TestDistributor_CloseLoopbackOutputs_Idempotent` |
| Raw inputs (AddRawInput) | ✅ | `TestEngine_GracefulShutdown_RawInput` |
| Raw outputs (AddRawOutput) | ✅ | `TestEngine_GracefulShutdown_RawOutput` |
| Wrapper goroutine cleanup | ✅ | `TestEngine_WrapperGoroutineCleanup` |
| Context cancelled before any message | ✅ | `TestEngine_GracefulShutdown_ImmediateCancel` |
| Infinite loop handler | ✅ | `TestEngine_GracefulShutdown_Timeout` |
| ProcessLoopback with transformation | ✅ | `TestEngine_GracefulShutdown_ProcessLoopback` |

### Convention Compliance

| Convention | Status | Notes |
|------------|--------|-------|
| Config struct for constructors | ✅ | MessageTracker uses simple constructor (no config needed) |
| Direct parameters for methods | ✅ | Enter(), Exit(), Close() have no parameters |
| Error wrapping | N/A | No errors returned from tracker |
| Godoc standards | ✅ | Included in API design section |
| Test naming | ✅ | Follows `TestType_Method_Scenario` pattern |
| Plan structure | ✅ | Follows planning.md template |

### Idiomatic Go Verification

| Aspect | Status | Notes |
|--------|--------|-------|
| Context propagation | ✅ | Uses existing ctx pattern from Engine |
| Channel closure for completion | ✅ | Drained() returns receive-only channel |
| WaitGroup for goroutine sync | ✅ | Engine tracks wrapper goroutines |
| Atomic for counters | ✅ | Uses atomic.Int64, atomic.Bool |
| sync.Once for one-time close | ✅ | Prevents double-close of drained channel |

## Testing Strategy

Tests must be written **before** implementation to establish baseline behavior and detect regressions.

### Phase 1: Baseline Tests (Pre-Implementation)

Establish current behavior and performance baselines.

#### 1.1 Shutdown Behavior Tests

Create `message/engine_shutdown_test.go`:

```go
func TestEngine_ShutdownWithoutLoopback(t *testing.T)
    // Baseline: Engine without loopback shuts down cleanly
    // Measures: shutdown latency, message delivery guarantee

func TestEngine_ShutdownWithLoopback_CurrentBehavior(t *testing.T)
    // Documents current deadlock/timeout behavior
    // Expected: Waits for ShutdownTimeout (slow)

func TestEngine_LoopbackMessageDelivery(t *testing.T)
    // Verifies all messages are delivered before shutdown
    // Baseline for message loss detection
```

#### 1.2 Performance Benchmarks

Create `message/engine_bench_test.go`:

```go
func BenchmarkEngine_Throughput_NoLoopback(b *testing.B)
    // Baseline throughput without loopback
    // Measures: messages/second

func BenchmarkEngine_Throughput_WithLoopback(b *testing.B)
    // Baseline throughput with loopback
    // Measures: messages/second

func BenchmarkEngine_ShutdownLatency_NoLoopback(b *testing.B)
    // Baseline shutdown time without loopback
    // Measures: time from cancel to done

func BenchmarkEngine_ShutdownLatency_WithLoopback(b *testing.B)
    // Current shutdown time (expect: ~ShutdownTimeout)
    // Measures: time from cancel to done

func BenchmarkEngine_MessageOverhead(b *testing.B)
    // Baseline per-message overhead
    // Measures: allocations, CPU time per message
```

#### 1.3 Component Benchmarks

Create `pipe/distributor_bench_test.go`:

```go
func BenchmarkDistributor_Route(b *testing.B)
    // Baseline routing performance

func BenchmarkDistributor_Route_WithLoopbackCheck(b *testing.B)
    // Simulates overhead of loopback output tracking
```

Create `pipe/tracker_bench_test.go`:

```go
func BenchmarkMessageTracker_EnterExit(b *testing.B)
    // Atomic counter overhead

func BenchmarkMessageTracker_EnterExit_Parallel(b *testing.B)
    // Contention under high concurrency
```

### Phase 2: Implementation Tests

Written alongside implementation, run against new code.

#### 2.1 MessageTracker Unit Tests

Create `pipe/tracker_test.go`:

```go
func TestMessageTracker_EnterExit(t *testing.T)
    // Basic counting

func TestMessageTracker_Drained_ClosesWhenZeroAndClosed(t *testing.T)
    // Drained channel closes when count=0 AND Close() called

func TestMessageTracker_Drained_NotClosedUntilClose(t *testing.T)
    // Count at 0 but Close() not called - Drained stays open

func TestMessageTracker_Drained_OnlyClosesOnce(t *testing.T)
    // Multiple zero crossings don't panic

func TestMessageTracker_Concurrent(t *testing.T)
    // Race detector clean under concurrent access

func TestMessageTracker_NegativeCount(t *testing.T)
    // Count can go negative (multiply case), Drained still works

func TestMessageTracker_CloseWithZeroCount(t *testing.T)
    // Close() when count already 0 - Drained closes immediately

func TestMessageTracker_CloseIdempotent(t *testing.T)
    // Multiple Close() calls are safe
```

#### 2.2 Distributor Loopback Tests

Add to `pipe/distributor_test.go`:

```go
func TestDistributor_AddLoopbackOutput(t *testing.T)
    // Loopback output is tracked separately

func TestDistributor_AddLoopbackOutput_AfterStart(t *testing.T)
    // Can add loopback output after Distribute() called

func TestDistributor_CloseLoopbackOutputs(t *testing.T)
    // Only loopback outputs are closed

func TestDistributor_CloseLoopbackOutputs_Idempotent(t *testing.T)
    // Safe to call multiple times

func TestDistributor_CloseLoopbackOutputs_WhileRouting(t *testing.T)
    // Safe to call during active routing

func TestDistributor_MixedOutputs(t *testing.T)
    // External and loopback outputs work together
```

#### 2.3 Engine Integration Tests

Add to `message/engine_shutdown_test.go`:

```go
func TestEngine_GracefulShutdown_SimpleLoopback(t *testing.T)
    // Single loopback, fast shutdown

func TestEngine_GracefulShutdown_BatchLoopback(t *testing.T)
    // BatchLoopback drains correctly

func TestEngine_GracefulShutdown_GroupLoopback(t *testing.T)
    // GroupLoopback drains correctly

func TestEngine_GracefulShutdown_ProcessLoopback(t *testing.T)
    // ProcessLoopback with channel.Process transformation

func TestEngine_GracefulShutdown_MultipleLoopbacks(t *testing.T)
    // Multiple independent loopbacks

func TestEngine_GracefulShutdown_ChainedLoopbacks(t *testing.T)
    // L1 output feeds L2 input

func TestEngine_GracefulShutdown_HandlerDropsMessages(t *testing.T)
    // Handler returns 0 messages

func TestEngine_GracefulShutdown_HandlerMultipliesMessages(t *testing.T)
    // Handler returns N > 1 messages

func TestEngine_GracefulShutdown_NoExternalInputs(t *testing.T)
    // Only loopback inputs (edge case)

func TestEngine_GracefulShutdown_NoMessages(t *testing.T)
    // Shutdown before any messages sent

func TestEngine_GracefulShutdown_ImmediateCancel(t *testing.T)
    // Context cancelled immediately after Start()

func TestEngine_GracefulShutdown_Timeout(t *testing.T)
    // Infinite loop handler triggers timeout

func TestEngine_GracefulShutdown_NoMessageLoss(t *testing.T)
    // All in-flight messages delivered

func TestEngine_GracefulShutdown_RawInput(t *testing.T)
    // AddRawInput is tracked

func TestEngine_GracefulShutdown_RawOutput(t *testing.T)
    // AddRawOutput is tracked

func TestEngine_ShutdownBeforeStart(t *testing.T)
    // Cancel context before Start() - no panic

func TestEngine_WrapperGoroutineCleanup(t *testing.T)
    // No goroutine leaks after shutdown
```

#### 2.4 Handler Wrapper Tests

Create `message/tracked_handler_test.go`:

```go
func TestTrackedHandler_OneToOne(t *testing.T)
    // Handler returns 1 message - no count change

func TestTrackedHandler_Drop(t *testing.T)
    // Handler returns 0 - Exit() called

func TestTrackedHandler_Multiply(t *testing.T)
    // Handler returns N - Enter() called N-1 times

func TestTrackedHandler_Error(t *testing.T)
    // Handler returns error - Exit() called

func TestTrackedHandler_PreservesEventType(t *testing.T)
    // EventType() delegated to inner handler

func TestTrackedHandler_PreservesNewInput(t *testing.T)
    // NewInput() delegated to inner handler
```

### Phase 3: Performance Validation

Run after implementation to verify overhead is acceptable.

#### 3.1 Overhead Analysis

```go
func BenchmarkEngine_Throughput_WithTracking(b *testing.B)
    // Compare to baseline - target: <5% overhead

func BenchmarkEngine_ShutdownLatency_WithTracking(b *testing.B)
    // Compare to baseline - target: <100ms for drained pipeline

func BenchmarkEngine_ChannelWrapper_Overhead(b *testing.B)
    // Measure wrapper goroutine overhead
    // Compare: direct channel vs wrapped channel
```

#### 3.2 Acceptance Criteria

| Metric | Baseline | Target | Acceptable |
|--------|----------|--------|------------|
| Throughput overhead | 0% | <2% | <5% |
| Per-message allocations | 0 extra | 0 extra | 1 extra |
| Shutdown latency (drained) | N/A | <50ms | <100ms |
| Shutdown latency (timeout) | ShutdownTimeout | ShutdownTimeout | Same |

### Phase 4: Stress Tests

```go
func TestEngine_Stress_HighThroughput(t *testing.T)
    // 100k messages, verify no race conditions

func TestEngine_Stress_RapidShutdown(t *testing.T)
    // Start/stop cycles, verify no goroutine leaks

func TestEngine_Stress_ConcurrentLoopbacks(t *testing.T)
    // Multiple loopbacks under high load
```

## Implementation Plan

### Task 1: Baseline Tests & Benchmarks

**Goal:** Establish current behavior before changes.

**Files:**
- `message/engine_shutdown_test.go` - Shutdown behavior tests
- `message/engine_bench_test.go` - Engine benchmarks
- `pipe/distributor_bench_test.go` - Distributor benchmarks

**Acceptance:**
- [ ] All baseline tests pass
- [ ] Benchmarks produce stable results
- [ ] Current loopback shutdown behavior documented

### Task 2: MessageTracker

**Goal:** Implement standalone counter with Close() semantics.

**Files:**
- `pipe/tracker.go` - Implementation
- `pipe/tracker_test.go` - Unit tests
- `pipe/tracker_bench_test.go` - Benchmarks

**Implementation:**

```go
// MessageTracker tracks in-flight messages for graceful shutdown.
type MessageTracker struct {
    inFlight atomic.Int64
    closed   atomic.Bool
    drained  chan struct{}
    once     sync.Once
}

func NewMessageTracker() *MessageTracker {
    return &MessageTracker{
        drained: make(chan struct{}),
    }
}

func (t *MessageTracker) Enter() {
    t.inFlight.Add(1)
}

func (t *MessageTracker) Exit() {
    if t.inFlight.Add(-1) == 0 && t.closed.Load() {
        t.once.Do(func() { close(t.drained) })
    }
}

func (t *MessageTracker) Close() {
    if t.closed.CompareAndSwap(false, true) {
        if t.inFlight.Load() == 0 {
            t.once.Do(func() { close(t.drained) })
        }
    }
}

func (t *MessageTracker) Drained() <-chan struct{} {
    return t.drained
}

func (t *MessageTracker) InFlight() int64 {
    return t.inFlight.Load()
}
```

**Acceptance:**
- [ ] All unit tests pass
- [ ] Race detector clean
- [ ] Benchmark shows <10ns per Enter/Exit
- [ ] Close() + zero count closes Drained() immediately

### Task 3: Distributor Loopback Support

**Goal:** Add loopback output tracking with idempotent close.

**Files:**
- `pipe/distributor.go` - Add methods
- `pipe/distributor_test.go` - Add tests

**Implementation:**

```go
type Distributor[T any] struct {
    // ... existing fields ...
    loopbackOutputs map[chan T]struct{}
    loopbackClosed  bool
}

func (d *Distributor[T]) AddLoopbackOutput(matcher func(T) bool) (<-chan T, error) {
    d.mu.Lock()
    defer d.mu.Unlock()

    if d.closed {
        return nil, errors.New("distributor: closed")
    }

    ch := make(chan T, d.cfg.Buffer)
    d.outputs = append(d.outputs, outputEntry[T]{ch: ch, matcher: matcher})

    if d.loopbackOutputs == nil {
        d.loopbackOutputs = make(map[chan T]struct{})
    }
    d.loopbackOutputs[ch] = struct{}{}

    return ch, nil
}

func (d *Distributor[T]) CloseLoopbackOutputs() {
    d.mu.Lock()
    defer d.mu.Unlock()

    if d.loopbackClosed {
        return  // Idempotent
    }
    d.loopbackClosed = true

    for ch := range d.loopbackOutputs {
        close(ch)
    }

    // Remove from outputs slice to prevent double-close in normal shutdown
    filtered := d.outputs[:0]
    for _, out := range d.outputs {
        if _, isLoopback := d.loopbackOutputs[out.ch]; !isLoopback {
            filtered = append(filtered, out)
        }
    }
    d.outputs = filtered
    d.loopbackOutputs = nil
}
```

**Acceptance:**
- [ ] `AddLoopbackOutput` works before and after Distribute()
- [ ] `CloseLoopbackOutputs` only closes loopback outputs
- [ ] `CloseLoopbackOutputs` is idempotent
- [ ] Existing tests still pass
- [ ] No performance regression in routing benchmark

### Task 4: Engine Channel Wrappers

**Goal:** Wrap input/output channels for tracking with proper goroutine lifecycle.

**Files:**
- `message/engine.go` - Add wrapper methods
- `message/tracking.go` - Wrapper implementations

**Implementation:**

```go
type Engine struct {
    // ... existing fields ...
    tracker   *pipe.MessageTracker
    wrapperWg sync.WaitGroup  // Track wrapper goroutines
}

func (e *Engine) wrapInputChannel(ch <-chan *Message) <-chan *Message {
    out := make(chan *Message)
    e.wrapperWg.Add(1)
    go func() {
        defer e.wrapperWg.Done()
        defer close(out)
        for msg := range ch {
            e.tracker.Enter()
            out <- msg
        }
    }()
    return out
}

func (e *Engine) wrapOutputChannel(ch <-chan *Message) <-chan *Message {
    out := make(chan *Message)
    e.wrapperWg.Add(1)
    go func() {
        defer e.wrapperWg.Done()
        defer close(out)
        for msg := range ch {
            e.tracker.Exit()
            out <- msg
        }
    }()
    return out
}

// AddLoopbackInput - no wrapping, direct to merger
func (e *Engine) AddLoopbackInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error) {
    e.cfg.Logger.Info("Adding loopback input", "input", name)
    filtered := e.applyTypedInputMatcher(name, ch, matcher)
    return e.merger.AddInput(filtered)
}

// AddLoopbackOutput - no wrapping, marked for shutdown
func (e *Engine) AddLoopbackOutput(name string, matcher Matcher) (<-chan *Message, error) {
    e.cfg.Logger.Info("Adding loopback output", "output", name)
    return e.distributor.AddLoopbackOutput(func(msg *Message) bool {
        return matcher == nil || matcher.Match(msg.Attributes)
    })
}
```

**Acceptance:**
- [ ] External channels are wrapped with Enter/Exit
- [ ] Loopback channels are not wrapped
- [ ] Raw input/output channels are wrapped
- [ ] Wrapper goroutines tracked via WaitGroup
- [ ] No goroutine leaks (verified via test)

### Task 5: Engine Handler Wrapper

**Goal:** Wrap handlers for drop/multiply tracking.

**Files:**
- `message/tracked_handler.go` - Implementation
- `message/tracked_handler_test.go` - Tests

**Implementation:**

```go
// trackedHandler wraps a Handler to track message drops and multiplies.
type trackedHandler struct {
    inner   Handler
    tracker *pipe.MessageTracker
}

func (h *trackedHandler) EventType() string {
    return h.inner.EventType()
}

func (h *trackedHandler) NewInput() any {
    return h.inner.NewInput()
}

func (h *trackedHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    results, err := h.inner.Handle(ctx, msg)
    if err != nil {
        h.tracker.Exit()  // Error = message consumed
        return nil, err
    }

    switch len(results) {
    case 0:
        h.tracker.Exit()  // Dropped
    case 1:
        // 1:1 transform, no change
    default:
        for i := 1; i < len(results); i++ {
            h.tracker.Enter()  // Multiplied
        }
    }
    return results, nil
}
```

**Acceptance:**
- [ ] Drop detection works (Exit called)
- [ ] Multiply detection works (Enter called N-1 times)
- [ ] Error handling works (Exit called)
- [ ] Handler interface fully preserved (EventType, NewInput)

### Task 6: Engine Shutdown Orchestration

**Goal:** Coordinate shutdown using tracker.

**Files:**
- `message/engine.go` - Update Start()

**Implementation:**

```go
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    // ... existing setup ...

    // Shutdown orchestration
    go func() {
        <-ctx.Done()

        // Signal no more external inputs expected
        e.tracker.Close()

        // Wait for pipeline to drain or timeout
        if e.cfg.ShutdownTimeout > 0 {
            select {
            case <-e.tracker.Drained():
                // Pipeline drained naturally
            case <-time.After(e.cfg.ShutdownTimeout):
                // Timeout - force close
            }
        } else {
            <-e.tracker.Drained()
        }

        // Close loopback outputs to break cycle
        e.distributor.CloseLoopbackOutputs()
    }()

    // ... start pipeline ...
}
```

**Acceptance:**
- [ ] Shutdown calls tracker.Close()
- [ ] Shutdown waits for Drained() or timeout
- [ ] Loopback outputs closed after drain
- [ ] All integration tests pass

### Task 7: Plugin Updates

**Goal:** Update loopback plugins to use new API.

**Files:**
- `message/plugin/loopback.go` - Use AddLoopbackInput/Output

**Implementation:**

```go
func Loopback(name string, matcher message.Matcher) message.Plugin {
    return func(e *message.Engine) error {
        out, err := e.AddLoopbackOutput(name, matcher)
        if err != nil {
            return fmt.Errorf("loopback output: %w", err)
        }
        _, err = e.AddLoopbackInput(name, nil, out)
        if err != nil {
            return fmt.Errorf("loopback input: %w", err)
        }
        return nil
    }
}

func ProcessLoopback(...) message.Plugin {
    return func(e *message.Engine) error {
        out, err := e.AddLoopbackOutput(name, matcher)
        if err != nil {
            return fmt.Errorf("process loopback output: %w", err)
        }
        processed := channel.Process(out, handle)
        _, err = e.AddLoopbackInput(name, nil, processed)
        if err != nil {
            return fmt.Errorf("process loopback input: %w", err)
        }
        return nil
    }
}

// BatchLoopback and GroupLoopback follow same pattern
```

**Acceptance:**
- [ ] All loopback plugin tests pass
- [ ] No API change for plugin users (`plugin.Loopback(...)` unchanged)
- [ ] ProcessLoopback with transformation works
- [ ] BatchLoopback works
- [ ] GroupLoopback works

### Task 8: Performance Validation

**Goal:** Verify overhead is acceptable.

**Acceptance:**
- [ ] Throughput overhead <5%
- [ ] Per-message allocations: 0 extra (or 1 acceptable)
- [ ] Shutdown latency <100ms for drained pipeline
- [ ] All stress tests pass
- [ ] No goroutine leaks under stress

## Implementation Order

```
Task 1: Baseline Tests ◄── Must complete first
    │
    ▼
Task 2: MessageTracker
    │
    ▼
Task 3: Distributor Loopback
    │
    ├──────────────────┐
    ▼                  ▼
Task 4: Channel     Task 5: Handler
Wrappers            Wrapper
    │                  │
    └────────┬─────────┘
             ▼
Task 6: Shutdown Orchestration
             │
             ▼
Task 7: Plugin Updates
             │
             ▼
Task 8: Performance Validation ◄── Must pass before merge
```

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Wrapper goroutine overhead | Performance regression | Benchmark before/after; WaitGroup tracks cleanup |
| Negative count edge cases | Drained never closes | Test multiply scenarios; Drained closes at 0 crossing when closed |
| Race in CloseLoopbackOutputs | Panic or deadlock | Proper locking; idempotent implementation; stress test |
| Handler wrapper breaks interface | Compilation errors | Test EventType/NewInput preservation |
| Close() not called | Drained never closes | Engine always calls Close() on ctx.Done() |

## Acceptance Criteria

- [ ] All Phase 1 baseline tests pass (before implementation)
- [ ] All Phase 2 implementation tests pass
- [ ] All Phase 3 performance benchmarks meet targets
- [ ] All Phase 4 stress tests pass
- [ ] Race detector clean (`go test -race`)
- [ ] No goroutine leaks
- [ ] CHANGELOG updated
