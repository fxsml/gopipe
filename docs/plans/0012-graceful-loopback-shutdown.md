# Plan: Graceful Loopback Shutdown

**Status:** Proposed
**Issue:** #81

## Overview

Enable graceful shutdown for pipelines with loopbacks without heuristics. Uses reference counting to detect when the pipeline is drained, then closes loopback outputs to break the cycle.

## Goals

1. Deterministic shutdown without idle-time heuristics
2. Works for all loopback topologies (simple, batch, group, chained)
3. No API changes for existing loopback plugins (internal refactor)
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
| Message exits to external output | `Exit()` | Engine output wrapper |
| Handler drops message (returns 0) | `Exit()` | Engine handler wrapper |
| Handler multiplies message (returns N>1) | `Enter()` × (N-1) | Engine handler wrapper |
| Message goes to loopback | Nothing | Not wrapped |
| Message returns from loopback | Nothing | Not wrapped |

### Shutdown Sequence

```
t0    ctx.Cancel()
t1    Wait for tracker.Drained() OR ShutdownTimeout
t2    Close loopback outputs (breaks cycle)
t3    Loopback inputs see closed channel
t4    Merger completes
t5    Pipeline drains naturally
t6    Engine done channel closes
```

## API Design

### Engine (public)

```go
// External - tracked
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error)
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error)

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
func (d *Distributor[T]) CloseLoopbackOutputs()
```

### MessageTracker (new, internal)

```go
type MessageTracker struct {
    inFlight atomic.Int64
    drained  chan struct{}
    once     sync.Once
}

func NewMessageTracker() *MessageTracker
func (t *MessageTracker) Enter()
func (t *MessageTracker) Exit()
func (t *MessageTracker) Drained() <-chan struct{}
func (t *MessageTracker) InFlight() int64
```

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

func TestMessageTracker_Drained_ClosesAtZero(t *testing.T)
    // Drained channel closes when count reaches 0

func TestMessageTracker_Drained_OnlyClosesOnce(t *testing.T)
    // Multiple zero crossings don't panic

func TestMessageTracker_Concurrent(t *testing.T)
    // Race detector clean under concurrent access

func TestMessageTracker_NegativeCount(t *testing.T)
    // Count can go negative (multiply case), Drained still works
```

#### 2.2 Distributor Loopback Tests

Add to `pipe/distributor_test.go`:

```go
func TestDistributor_AddLoopbackOutput(t *testing.T)
    // Loopback output is tracked separately

func TestDistributor_CloseLoopbackOutputs(t *testing.T)
    // Only loopback outputs are closed

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

func TestEngine_GracefulShutdown_Timeout(t *testing.T)
    // Infinite loop handler triggers timeout

func TestEngine_GracefulShutdown_NoMessageLoss(t *testing.T)
    // All in-flight messages delivered
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

func TestTrackedHandler_PreservesInterface(t *testing.T)
    // EventType(), NewInput() still work
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

**Goal:** Implement standalone counter.

**Files:**
- `pipe/tracker.go` - Implementation
- `pipe/tracker_test.go` - Unit tests
- `pipe/tracker_bench_test.go` - Benchmarks

**Acceptance:**
- [ ] All unit tests pass
- [ ] Race detector clean
- [ ] Benchmark shows <10ns per Enter/Exit

### Task 3: Distributor Loopback Support

**Goal:** Add loopback output tracking.

**Files:**
- `pipe/distributor.go` - Add methods
- `pipe/distributor_test.go` - Add tests

**Acceptance:**
- [ ] `AddLoopbackOutput` works
- [ ] `CloseLoopbackOutputs` only closes loopback outputs
- [ ] Existing tests still pass
- [ ] No performance regression in routing benchmark

### Task 4: Engine Channel Wrappers

**Goal:** Wrap input/output channels for tracking.

**Files:**
- `message/engine.go` - Add wrapper methods
- `message/tracking.go` - Wrapper implementations

**Acceptance:**
- [ ] External channels are wrapped
- [ ] Loopback channels are not wrapped
- [ ] No goroutine leaks (verified via test)

### Task 5: Engine Handler Wrapper

**Goal:** Wrap handlers for drop/multiply tracking.

**Files:**
- `message/tracked_handler.go` - Implementation
- `message/tracked_handler_test.go` - Tests

**Acceptance:**
- [ ] Drop detection works
- [ ] Multiply detection works
- [ ] Handler interface preserved

### Task 6: Engine Shutdown Orchestration

**Goal:** Coordinate shutdown using tracker.

**Files:**
- `message/engine.go` - Update Start()

**Acceptance:**
- [ ] Shutdown waits for Drained() or timeout
- [ ] Loopback outputs closed after drain
- [ ] All integration tests pass

### Task 7: Plugin Updates

**Goal:** Update loopback plugins to use new API.

**Files:**
- `message/plugin/loopback.go` - Use AddLoopbackInput/Output

**Acceptance:**
- [ ] All loopback plugin tests pass
- [ ] No API change for plugin users

### Task 8: Performance Validation

**Goal:** Verify overhead is acceptable.

**Acceptance:**
- [ ] Throughput overhead <5%
- [ ] Per-message allocations: 0 extra (or 1 acceptable)
- [ ] Shutdown latency <100ms for drained pipeline
- [ ] All stress tests pass

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
| Wrapper goroutine overhead | Performance regression | Benchmark before/after; consider sync approach if needed |
| Negative count edge cases | Drained never closes | Test multiply scenarios; Drained closes at 0 crossing |
| Race in CloseLoopbackOutputs | Panic or deadlock | Proper locking; stress test with race detector |
| Handler wrapper breaks interface | Compilation errors | Test EventType/NewInput preservation |

## Acceptance Criteria

- [ ] All Phase 1 baseline tests pass (before implementation)
- [ ] All Phase 2 implementation tests pass
- [ ] All Phase 3 performance benchmarks meet targets
- [ ] All Phase 4 stress tests pass
- [ ] Race detector clean (`go test -race`)
- [ ] No goroutine leaks
- [ ] CHANGELOG updated
