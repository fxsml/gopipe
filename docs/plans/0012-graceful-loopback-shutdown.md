# Plan: Graceful Loopback Shutdown

**Status:** Proposed
**Issue:** #81

## Overview

Enable graceful shutdown for pipelines with loopbacks without heuristics. Uses reference counting to detect when the pipeline is drained, then closes loopback outputs to break the cycle.

## Goals

1. Deterministic shutdown without idle-time heuristics
2. Works for simple and complex loopback topologies
3. No API changes for existing loopback plugins
4. Involves Distributor in shutdown coordination

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

## Solution: Reference Counting

Track messages through the pipeline. When external inputs close AND in-flight count reaches zero, the pipeline is drained. Then close loopback outputs to break the cycle.

**Why this works for all cases:**
- Simple loopback: Count decreases as messages exit to external outputs
- Multiple loopbacks: Same counting, all drain together
- Chained loopbacks: Messages flow through, count tracks total in-flight
- Mutual loopbacks: Count only reaches zero when cycle is empty

### MessageTracker

```go
type MessageTracker struct {
    inFlight atomic.Int64
    drained  chan struct{}
    once     sync.Once
}

func (t *MessageTracker) Enter() {
    t.inFlight.Add(1)
}

func (t *MessageTracker) Exit() {
    if t.inFlight.Add(-1) == 0 {
        t.once.Do(func() { close(t.drained) })
    }
}

func (t *MessageTracker) Drained() <-chan struct{} {
    return t.drained
}
```

**Counting rules:**
- `Enter()`: Called when message enters Merger from external input
- `Exit()`: Called when message exits Distributor to external output
- Loopback: Message re-enters Merger, but no new `Enter()` (already counted)

### Shutdown Sequence

```
Time    Event
─────   ──────────────────────────────────────
t0      ctx.Cancel() called
t1      External inputs close (user responsibility)
t2      Wait for tracker.Drained() OR ShutdownTimeout
t3      Close loopback outputs (breaks cycle)
t4      Loopback inputs detect closed channels
t5      Merger completes (all inputs closed)
t6      Pipeline drains naturally
t7      Engine done channel closes
```

## Tasks

### Task 1: Add MessageTracker

**Goal:** Track in-flight messages through the pipeline.

**Implementation:**

```go
// pipe/tracker.go
package pipe

type MessageTracker struct {
    inFlight atomic.Int64
    drained  chan struct{}
    once     sync.Once
}

func NewMessageTracker() *MessageTracker {
    return &MessageTracker{
        drained: make(chan struct{}),
    }
}

func (t *MessageTracker) Enter() { t.inFlight.Add(1) }

func (t *MessageTracker) Exit() {
    if t.inFlight.Add(-1) == 0 {
        t.once.Do(func() { close(t.drained) })
    }
}

func (t *MessageTracker) Drained() <-chan struct{} { return t.drained }

func (t *MessageTracker) InFlight() int64 { return t.inFlight.Load() }
```

**Files to Create:**
- `pipe/tracker.go` - MessageTracker implementation
- `pipe/tracker_test.go` - Unit tests

**Acceptance Criteria:**
- [ ] Enter/Exit correctly track count
- [ ] Drained() closes when count reaches zero
- [ ] Thread-safe under concurrent access

### Task 2: Add Loopback Tracking to Distributor

**Goal:** Distributor tracks which outputs are loopbacks and can close them on demand.

**Implementation:**

```go
type Distributor[T any] struct {
    // ... existing fields ...
    loopbackOutputs map[chan T]struct{}
}

func (d *Distributor[T]) MarkLoopbackOutput(ch <-chan T) {
    d.mu.Lock()
    defer d.mu.Unlock()
    // Find the internal channel that matches this read-only channel
    for _, out := range d.outputs {
        if (<-chan T)(out.ch) == ch {
            if d.loopbackOutputs == nil {
                d.loopbackOutputs = make(map[chan T]struct{})
            }
            d.loopbackOutputs[out.ch] = struct{}{}
            return
        }
    }
}

func (d *Distributor[T]) CloseLoopbackOutputs() {
    d.mu.Lock()
    defer d.mu.Unlock()
    for ch := range d.loopbackOutputs {
        close(ch)
    }
    // Remove from outputs to prevent double-close
    d.loopbackOutputs = nil
}

func (d *Distributor[T]) IsLoopbackOutput(ch chan T) bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    _, ok := d.loopbackOutputs[ch]
    return ok
}
```

**Files to Modify:**
- `pipe/distributor.go` - Add loopback tracking

**Acceptance Criteria:**
- [ ] MarkLoopbackOutput correctly identifies loopback channels
- [ ] CloseLoopbackOutputs closes only loopback outputs
- [ ] Non-loopback outputs remain open

### Task 3: Integrate Tracker with Distributor

**Goal:** Distributor calls Exit() for external outputs only.

**Implementation:**

```go
type DistributorConfig[T any] struct {
    // ... existing fields ...
    Tracker *MessageTracker
}

func (d *Distributor[T]) route(in T) {
    // ... existing matching logic ...

    for _, out := range outputs {
        if out.matcher == nil || out.matcher(in) {
            select {
            case out.ch <- in:
                // Track exit for non-loopback outputs
                if d.cfg.Tracker != nil && !d.IsLoopbackOutput(out.ch) {
                    d.cfg.Tracker.Exit()
                }
            case <-d.done:
                d.cfg.ErrorHandler(in, ErrShutdownDropped)
            }
            return
        }
    }
    // ... no match handling ...
}
```

**Files to Modify:**
- `pipe/distributor.go` - Integrate tracker

**Acceptance Criteria:**
- [ ] Exit() called for external outputs
- [ ] Exit() NOT called for loopback outputs
- [ ] Tracker optional (nil check)

### Task 4: Integrate Tracker with Merger

**Goal:** Merger calls Enter() for external inputs only.

**Implementation:**

```go
type MergerConfig struct {
    // ... existing fields ...
    Tracker *MessageTracker
}

func (m *Merger[T]) AddInput(ch <-chan T) (<-chan struct{}, error) {
    return m.addInput(ch, false)
}

func (m *Merger[T]) AddLoopbackInput(ch <-chan T) (<-chan struct{}, error) {
    return m.addInput(ch, true)
}

func (m *Merger[T]) addInput(ch <-chan T, isLoopback bool) (<-chan struct{}, error) {
    // ... existing logic ...
    // Store isLoopback flag with input
}

func (m *Merger[T]) startInput(ch <-chan T, done chan struct{}, isLoopback bool) {
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        if done != nil {
            defer close(done)
        }
        for {
            select {
            case <-m.done:
                return
            case v, ok := <-ch:
                if !ok {
                    return
                }
                // Track entry for non-loopback inputs
                if m.cfg.Tracker != nil && !isLoopback {
                    m.cfg.Tracker.Enter()
                }
                select {
                case m.out <- v:
                case <-m.done:
                    m.cfg.ErrorHandler(v, ErrShutdownDropped)
                    return
                }
            }
        }
    }()
}
```

**Files to Modify:**
- `pipe/merger.go` - Add AddLoopbackInput, integrate tracker

**Acceptance Criteria:**
- [ ] Enter() called for external inputs
- [ ] Enter() NOT called for loopback inputs
- [ ] Tracker optional (nil check)

### Task 5: Engine Shutdown Orchestration

**Goal:** Engine coordinates shutdown using tracker.

**Implementation:**

```go
type Engine struct {
    // ... existing fields ...
    tracker *pipe.MessageTracker
}

func NewEngine(cfg EngineConfig) *Engine {
    tracker := pipe.NewMessageTracker()

    e := &Engine{
        tracker: tracker,
        merger: pipe.NewMerger[*Message](pipe.MergerConfig{
            Buffer:          cfg.BufferSize,
            ShutdownTimeout: cfg.ShutdownTimeout,
            Tracker:         tracker,
        }),
        distributor: pipe.NewDistributor(pipe.DistributorConfig[*Message]{
            Buffer:  cfg.BufferSize,
            Tracker: tracker,
            // ... error handler ...
        }),
    }
    return e
}

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    // ... existing setup ...

    go func() {
        <-ctx.Done()

        // Wait for pipeline to drain or timeout
        select {
        case <-e.tracker.Drained():
            // Pipeline drained naturally
        case <-time.After(e.cfg.ShutdownTimeout):
            // Timeout - force close
        }

        // Close loopback outputs to break cycle
        e.distributor.CloseLoopbackOutputs()
    }()

    // ... start pipeline ...
}
```

**Files to Modify:**
- `message/engine.go` - Create tracker, orchestrate shutdown

**Acceptance Criteria:**
- [ ] Tracker created and wired to merger/distributor
- [ ] Shutdown waits for drain or timeout
- [ ] Loopback outputs closed after drain

### Task 6: Update Loopback Plugin

**Goal:** Loopback plugin marks channels for shutdown coordination.

**Implementation:**

```go
func Loopback(name string, matcher message.Matcher) message.Plugin {
    return func(e *message.Engine) error {
        out, err := e.AddOutput(name, matcher)
        if err != nil {
            return fmt.Errorf("loopback output: %w", err)
        }

        // Mark as loopback for shutdown coordination
        e.MarkLoopbackOutput(out)

        _, err = e.AddLoopbackInput(name, nil, out)
        if err != nil {
            return fmt.Errorf("loopback input: %w", err)
        }
        return nil
    }
}
```

**Engine methods:**

```go
func (e *Engine) MarkLoopbackOutput(ch <-chan *Message) {
    e.distributor.MarkLoopbackOutput(ch)
}

func (e *Engine) AddLoopbackInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error) {
    e.cfg.Logger.Info("Adding loopback input", "input", name)
    filtered := e.applyTypedInputMatcher(name, ch, matcher)
    return e.merger.AddLoopbackInput(filtered)
}
```

**Files to Modify:**
- `message/engine.go` - Add MarkLoopbackOutput, AddLoopbackInput
- `message/plugin/loopback.go` - Use new methods

**Acceptance Criteria:**
- [ ] Loopback plugin uses MarkLoopbackOutput
- [ ] Loopback plugin uses AddLoopbackInput
- [ ] ProcessLoopback, BatchLoopback, GroupLoopback updated

### Task 7: Testing

**Goal:** Verify shutdown works correctly for all scenarios.

**Test cases:**
1. Simple loopback - fast shutdown
2. Multiple independent loopbacks
3. Chained loopbacks (L1 output → L2 input)
4. No loopbacks (baseline)
5. Shutdown timeout triggers force-close
6. Infinite loop detection (handler always loops)

**Files to Create:**
- `pipe/tracker_test.go` - Tracker unit tests
- `message/engine_shutdown_test.go` - Integration tests

**Acceptance Criteria:**
- [ ] All test cases pass
- [ ] Race detector clean
- [ ] Shutdown latency < 100ms for drained pipeline

## Implementation Order

```
Task 1: MessageTracker
    ↓
Task 2: Distributor loopback tracking
    ↓
Task 3: Distributor + Tracker integration
    ↓
Task 4: Merger + Tracker integration
    ↓
Task 5: Engine orchestration
    ↓
Task 6: Plugin updates
    ↓
Task 7: Testing
```

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| No external inputs | Drained() returns immediately |
| No loopbacks | Tracker still works, just counts external flow |
| Infinite loop handler | Timeout triggers, force-close loopbacks |
| Messages in buffer at shutdown | Delivered before outputs close |
| Concurrent AddInput during shutdown | Rejected (merger closed) |

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass (`make test`)
- [ ] Race detector clean
- [ ] CHANGELOG updated
- [ ] Shutdown latency improved (benchmark)
