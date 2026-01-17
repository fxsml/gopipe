# Plan: Graceful Loopback Shutdown

**Issue:** #81
**Status:** Draft
**Author:** Claude
**Date:** 2026-01-17

## Problem Statement

When using loopback plugins, the engine experiences a shutdown deadlock. The circular dependency is:

```
                   ┌─────────────────────────────────────────┐
                   │                                         │
                   ↓                                         │
External Inputs → Merger → Router → Distributor → Loopback Output ─┘
                                          │
                                          └→ External Output
```

**Shutdown deadlock sequence:**
1. Context is cancelled
2. Merger waits for all inputs to close (or timeout)
3. Loopback input can only close when its source (Distributor output) closes
4. Distributor waits for its input to close (Router output)
5. Router waits for Merger output to close
6. Merger won't close output until all inputs close
7. **DEADLOCK**

Currently, only `ShutdownTimeout` breaks this cycle, forcing users to wait the full timeout duration.

## Analysis of Issue #81 Proposed Solution

The proposal adds `AddLoopbackInput` with drain detection using:
- `DrainTimeout`: Maximum wait for loopbacks to drain
- `DrainIdleTime`: Idle period threshold (heuristic)

### Pros
- Works without requiring changes to existing user code
- Relatively simple to implement

### Cons
- **Uses heuristics** - fragile and can fail with slow processing
- **Adds API complexity** - new method `AddLoopbackInput`, new config parameters
- **No Distributor involvement** - doesn't leverage that Distributor knows its outputs
- **Can't distinguish "idle" from "slow"** - a slow handler could trigger false drain detection

## Alternative Solutions Analysis

### Option 1: Keep Heuristics (Issue #81 Proposal)
**Verdict:** Works but fragile. Not recommended for production-critical systems.

### Option 2: User-Managed Done Channel
```go
loopback, closeFn := engine.CreateLoopbackWithClose("name", matcher)
defer closeFn() // User signals when done
```
**Verdict:** Puts burden on user, error-prone, violates "simple API" goal.

### Option 3: Reference Counting (Track In-Flight Messages)
Track active messages through the pipeline. When external inputs close AND in-flight count reaches zero, loopback is done.

**Verdict:** Complex implementation, runtime overhead, hard to get right with concurrent processing.

### Option 4: Engine-Managed Loopback with Distributor Coordination (Recommended)
The Engine and Distributor coordinate the shutdown:
1. Engine knows which inputs are loopbacks
2. Distributor knows which outputs are loopbacks
3. On shutdown, Engine orchestrates phased closing

**Verdict:** Clean, deterministic, no heuristics, involves Distributor.

## Recommended Solution: Engine-Coordinated Loopback Shutdown

### Core Insight

The key insight is that **we can deterministically close loopback outputs** when external inputs are done. This breaks the cycle without heuristics.

**Shutdown phases:**
1. **Phase 1:** Wait for external inputs to close (or timeout)
2. **Phase 2:** Close loopback outputs in Distributor (breaks the cycle)
3. **Phase 3:** Loopback inputs drain naturally (their source is closed)
4. **Phase 4:** Merger completes, closes output
5. **Phase 5:** Rest of pipeline drains naturally

### API Design

#### Option A: Minimal API Change (Recommended)

Keep the existing plugin API unchanged. Add internal tracking:

```go
// Loopback plugin remains unchanged - simple!
plugin.Loopback("name", matcher)

// Internally, Engine tracks the connection
```

**User experience:** No API change for loopback creation. Shutdown just works.

#### Option B: Explicit Loopback Registration

```go
// New method that creates matched pair
loopbackCh := engine.CreateLoopback("name", inputMatcher, outputMatcher)
```

**User experience:** Simpler than AddOutput+AddInput, but changes existing plugin pattern.

### Implementation Approach (Option A)

#### Step 1: Add Loopback Output Tracking to Distributor

```go
type Distributor[T any] struct {
    // ... existing fields ...
    loopbackOutputs map[chan T]struct{} // Track loopback outputs
}

// Mark an output as a loopback (can be closed early during shutdown)
func (d *Distributor[T]) MarkLoopbackOutput(ch <-chan T)

// Close all loopback outputs (called by Engine during shutdown)
func (d *Distributor[T]) CloseLoopbackOutputs()
```

#### Step 2: Add Loopback Input Tracking to Merger

```go
type Merger[T any] struct {
    // ... existing fields ...
    loopbackInputs map[<-chan T]struct{} // Track loopback inputs
}

// Mark an input as a loopback (doesn't block shutdown)
func (m *Merger[T]) MarkLoopbackInput(ch <-chan T)

// Returns true when all non-loopback inputs are closed
func (m *Merger[T]) ExternalInputsDone() <-chan struct{}
```

#### Step 3: Engine Coordinates Shutdown

```go
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    // ... existing setup ...

    go func() {
        <-ctx.Done()

        // Phase 1: Wait for external inputs to close (or timeout)
        select {
        case <-e.merger.ExternalInputsDone():
            // External inputs done naturally
        case <-time.After(e.cfg.ShutdownTimeout):
            // Timeout - force merger shutdown
        }

        // Phase 2: Close loopback outputs to break the cycle
        e.distributor.CloseLoopbackOutputs()

        // Phase 3-5: Pipeline drains naturally
    }()
}
```

#### Step 4: Update Loopback Plugin to Register

```go
func Loopback(name string, matcher message.Matcher) message.Plugin {
    return func(e *message.Engine) error {
        out, err := e.AddOutput(name, matcher)
        if err != nil {
            return fmt.Errorf("loopback output: %w", err)
        }

        // NEW: Mark as loopback for shutdown coordination
        e.MarkLoopbackOutput(out)

        _, err = e.AddInput(name, nil, out)
        if err != nil {
            return fmt.Errorf("loopback input: %w", err)
        }

        // NEW: Mark input as loopback
        e.MarkLoopbackInput(out)

        return nil
    }
}
```

### Detailed Shutdown Sequence

```
Time    Event
─────   ──────────────────────────────────────────────────
t0      ctx.Cancel() called
t1      Merger marks closed=true, starts shutdown timer
t2      External input channels close
t3      External input goroutines finish (wg decrements)
t4      Merger detects ExternalInputsDone()
t5      Engine calls distributor.CloseLoopbackOutputs()
t6      Loopback output channels close
t7      Loopback input goroutines see closed channel, finish
t8      All merger input goroutines done (wg.Wait() returns)
t9      Merger closes output channel
t10     Router drains remaining messages
t11     Router closes output
t12     Distributor drains remaining messages
t13     Distributor closes all outputs
t14     Marshal pipes drain
t15     Engine done channel closes
```

### Edge Cases

#### 1. No External Inputs (Loopback Only)
If the only inputs are loopbacks, `ExternalInputsDone()` returns immediately. The loopback outputs are closed, breaking the cycle.

#### 2. Messages In-Flight at Shutdown
Messages already in the loopback output buffer are processed. Messages in the merger that came from loopback are delivered to router. No message loss for in-flight messages.

#### 3. ShutdownTimeout Expires Before External Inputs Close
If external inputs don't close within timeout, the merger force-closes (existing behavior). Then loopback outputs are closed, allowing clean drain.

#### 4. Multiple Loopbacks
All loopback outputs are closed simultaneously. Each loopback input drains independently.

#### 5. Nested Loopbacks (Loopback → Process → Loopback)
Each segment is tracked independently. Closing happens in order as the pipeline drains.

### Alternative: Done Channel Approach (No Timeout)

For users who want deterministic shutdown without any timeout:

```go
type EngineConfig struct {
    // ...
    // If true, loopback shutdown is triggered by closing external inputs
    // rather than by ShutdownTimeout. Context cancel only stops accepting
    // new messages; shutdown completes when pipeline drains.
    GracefulLoopbackShutdown bool
}
```

With this option:
1. User closes external input channels
2. Engine detects all external inputs closed
3. Engine closes loopback outputs
4. Pipeline drains completely
5. Context cancel is only needed if user wants to force-stop

This provides **fully deterministic shutdown** with no timeouts or heuristics.

## API Comparison

### Current (with loopback deadlock)
```go
engine.AddPlugin(plugin.Loopback("loop", matcher))
// Shutdown: must wait full ShutdownTimeout
```

### Proposed (Option A - Minimal Change)
```go
engine.AddPlugin(plugin.Loopback("loop", matcher))
// Shutdown: fast, deterministic (plugin unchanged!)
```

### Alternative (Option B - Explicit)
```go
engine.CreateLoopback("loop", matcher)
// Shutdown: fast, deterministic
```

## Implementation Plan

### Phase 1: Distributor Changes
1. Add `loopbackOutputs` tracking map
2. Add `MarkLoopbackOutput(ch)` method
3. Add `CloseLoopbackOutputs()` method
4. Update shutdown sequence to handle partial output closure

### Phase 2: Merger Changes
1. Add `loopbackInputs` tracking map
2. Add `MarkLoopbackInput(ch)` method
3. Add `ExternalInputsDone()` channel
4. Track external vs loopback input completion separately

### Phase 3: Engine Changes
1. Add `MarkLoopbackOutput(ch)` passthrough to distributor
2. Add `MarkLoopbackInput(ch)` passthrough to merger
3. Update shutdown goroutine to coordinate phases
4. Add tests for shutdown scenarios

### Phase 4: Plugin Updates
1. Update `Loopback` to call mark methods
2. Update `ProcessLoopback` to call mark methods
3. Update `BatchLoopback` to call mark methods
4. Update `GroupLoopback` to call mark methods

### Phase 5: Documentation
1. Update README with shutdown behavior
2. Add examples for graceful shutdown
3. Document the shutdown phases

## Testing Strategy

1. **Unit tests:** Each component's new methods
2. **Integration tests:** Full engine with loopback, verify fast shutdown
3. **Edge case tests:** No external inputs, multiple loopbacks, timeout scenarios
4. **Benchmark:** Compare shutdown time before/after

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking change if output close order matters | Document behavior; outputs receive all queued messages |
| Race condition in close sequence | Use proper synchronization; add test for race detector |
| Existing code relies on full timeout | This is an improvement; no code depends on slow shutdown |

## Decision Points

1. **Option A vs B:** Recommend Option A (minimal API change) for backwards compatibility
2. **Timeout behavior:** Should `ShutdownTimeout` apply to external inputs only, or total shutdown?
   - Recommend: External inputs only; loopback closure is immediate after
3. **Error handling:** What if `CloseLoopbackOutputs` fails?
   - Recommend: Log warning, continue shutdown (best effort)

## Complex Pipeline Scenarios

The basic solution handles simple loopbacks, but complex pipelines require more sophisticated orchestration.

### Scenario 1: Multiple Independent Loopbacks

```
                    ┌── Loopback1 (retry errors) ──┐
                    │                               │
External Input → Merger → Router → Distributor ────┤
                    │                               │
                    └── Loopback2 (batch events) ──┘
```

**Challenge:** Both loopbacks must close, but they're independent.

**Solution:** Close all loopback outputs simultaneously when external inputs are done.

### Scenario 2: Chained Loopbacks

```
External → Merger → Router → Distributor → Loopback1 Output
              ↑                                  │
              │                                  ↓
              │                           ProcessPipe
              │                                  │
              └── Loopback2 Input ← Distributor2 ← Loopback1 Input
                                          │
                                          └→ Loopback2 Output ─┐
                                                               │
                                     ┌─────────────────────────┘
                                     ↓
                              (back to Merger)
```

**Challenge:** L2 depends on L1's output. Closing L1 first leaves L2 orphaned.

**Solution:** Track the full dependency graph, close in reverse topological order.

### Scenario 3: Mutual/Circular Loopbacks

```
              ┌──────────── Loopback1 ────────────┐
              │                                    │
              ↓                                    │
External → Merger → Router → Distributor ─────────┤
              ↑                                    │
              │                                    │
              └──────────── Loopback2 ────────────┘
```

Where L1 produces messages that L2 catches, AND L2 produces messages that L1 catches.

**Challenge:** No safe order to close—mutual dependency.

**Solution:** Use reference counting to detect when the cycle is "drained" (no in-flight messages), then close both simultaneously.

### Scenario 4: Pipeline of Engines

```
Engine1 → RawOutput → Engine2 → RawOutput → Engine3
   ↑          │          ↑          │
   └──────────┘          └──────────┘
   (loopback)            (loopback)
```

**Challenge:** Each engine has its own loopback; inter-engine communication adds latency.

**Solution:** Pipeline plugin coordinates shutdown across engines.

## Shutdown Orchestrator Design

To handle all scenarios, we need a proper **Shutdown Orchestrator** that:
1. Tracks the full pipeline topology
2. Detects cycles and dependencies
3. Coordinates phased shutdown
4. Handles edge cases gracefully

### Core Concept: Reference Counting

Track messages through the entire pipeline:

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

**How it works:**
- On entry to merger: `tracker.Enter()`
- On exit from distributor (to external output): `tracker.Exit()`
- Loopback messages: Don't exit—they re-enter
- When external inputs close AND in-flight count = 0: All messages processed

**This is deterministic and handles any topology!**

### Orchestrator Architecture

```go
type ShutdownOrchestrator struct {
    tracker     *MessageTracker
    merger      *pipe.Merger[*Message]
    distributor *pipe.Distributor[*Message]

    mu              sync.Mutex
    loopbackOutputs []chan *Message
    externalInputs  []<-chan *Message
}

func (o *ShutdownOrchestrator) Shutdown(ctx context.Context) error {
    // Phase 1: Stop accepting new external messages
    // (external inputs should be closed by user)

    // Phase 2: Wait for pipeline to drain
    select {
    case <-o.tracker.Drained():
        // All messages processed - clean shutdown
    case <-time.After(o.cfg.ShutdownTimeout):
        // Timeout - force shutdown
        o.forceCloseLoopbacks()
    case <-ctx.Done():
        // Cancelled - force shutdown
        o.forceCloseLoopbacks()
    }

    // Phase 3: Close loopback outputs (if not already)
    o.closeLoopbackOutputs()

    // Phase 4: Wait for pipeline to fully drain
    return o.waitForCompletion()
}
```

### Handling Infinite Loops

What if a handler creates an infinite loop (always produces loopback messages)?

```go
// Handler that loops forever
func (h *BadHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    return []*Message{msg}, nil // Always loops back!
}
```

**Detection:**
- If in-flight count doesn't decrease after ShutdownTimeout
- Log warning about potential infinite loop
- Force-close loopbacks

**Mitigation:**
- Optional `MaxLoopIterations` config
- TTL/hop-count middleware (already supported)

## Pipeline Plugin Design

For complex orchestration, provide a high-level **Pipeline Plugin** that declares the topology:

### API Design

```go
// Declarative pipeline with explicit stages
p := plugin.NewPipeline("order-processing")

// Stage 1: Validate incoming orders
p.Stage("validate",
    validateHandler,
)

// Stage 2: Enrich with customer data, retry on failure
p.Stage("enrich",
    enrichHandler,
    plugin.WithLoopback("retry", retryMatcher, plugin.MaxRetries(3)),
)

// Stage 3: Aggregate orders by customer
p.Stage("aggregate",
    aggregateHandler,
    plugin.WithBatchLoopback("batch", batchMatcher, batchFn, BatchConfig{
        MaxSize:     100,
        MaxDuration: time.Second,
    }),
)

// Stage 4: Publish to downstream
p.Stage("publish",
    publishHandler,
)

// Register with engine
engine.AddPlugin(p.Build())
```

### Benefits

1. **Clear structure:** Pipeline topology is explicit
2. **Automatic orchestration:** Plugin manages shutdown order
3. **Observability:** Can add per-stage metrics
4. **Validation:** Detect invalid topologies at registration
5. **Optimization:** Can optimize routing based on declared stages

### Internal Implementation

```go
type Pipeline struct {
    name   string
    stages []Stage
}

type Stage struct {
    name     string
    handler  Handler
    loopback *LoopbackConfig
}

func (p *Pipeline) Build() message.Plugin {
    return func(e *message.Engine) error {
        // 1. Register all handlers
        for _, stage := range p.stages {
            if err := e.AddHandler(stage.name, nil, stage.handler); err != nil {
                return err
            }
        }

        // 2. Register loopbacks in reverse order (for shutdown)
        // This ensures later stages close first
        for i := len(p.stages) - 1; i >= 0; i-- {
            stage := p.stages[i]
            if stage.loopback != nil {
                if err := e.AddLoopback(stage.loopback); err != nil {
                    return err
                }
            }
        }

        // 3. Register shutdown hook
        e.OnShutdown(p.orchestrateShutdown)

        return nil
    }
}

func (p *Pipeline) orchestrateShutdown(ctx context.Context, e *message.Engine) {
    // Close loopbacks in order: last stage first
    for i := len(p.stages) - 1; i >= 0; i-- {
        stage := p.stages[i]
        if stage.loopback != nil {
            e.CloseLoopback(stage.loopback.Name)
            // Wait for stage to drain
            <-stage.drained
        }
    }
}
```

### Automatic Topology Detection (Alternative)

For users who don't want to declare stages explicitly:

```go
// Engine automatically infers topology from registration order
engine.AddPlugin(plugin.Loopback("L1", m1))    // Registered first
engine.AddPlugin(plugin.Loopback("L2", m2))    // Registered second

// Shutdown closes in reverse order: L2 first, then L1
```

**How it works:**
- Track registration order of loopbacks
- Later registrations are "deeper" in the pipeline
- Close in reverse order (LIFO)

**Limitation:** Can't detect actual message flow dependencies.

## Updated Implementation Plan

### Phase 1: Message Tracking Infrastructure
1. Add `MessageTracker` struct with Enter/Exit/Drained
2. Integrate with Merger (Enter on input)
3. Integrate with Distributor (Exit on external output)
4. Add tests for tracking accuracy

### Phase 2: Distributor Loopback Support
1. Add `loopbackOutputs` tracking
2. Add `MarkLoopbackOutput(ch)` method
3. Add `CloseLoopbackOutputs()` method
4. Skip Exit() for loopback outputs in tracking

### Phase 3: Merger Loopback Support
1. Add `loopbackInputs` tracking
2. Add `MarkLoopbackInput(ch)` method
3. Add `ExternalInputsDone()` channel

### Phase 4: Shutdown Orchestrator
1. Create `ShutdownOrchestrator` struct
2. Implement phased shutdown logic
3. Add timeout and force-close handling
4. Integrate with Engine

### Phase 5: Engine Integration
1. Add `MarkLoopbackOutput/Input` passthrough methods
2. Create orchestrator in Engine.Start()
3. Wire up shutdown coordination
4. Add `OnShutdown` hook for plugins

### Phase 6: Plugin Updates
1. Update all loopback plugins to use mark methods
2. Add Pipeline plugin (optional, for complex topologies)
3. Update documentation

### Phase 7: Testing
1. Unit tests for each component
2. Integration tests for simple loopback
3. Integration tests for multiple loopbacks
4. Integration tests for chained loopbacks
5. Stress tests for race conditions
6. Benchmark shutdown latency

## Configuration Options

```go
type EngineConfig struct {
    // ... existing fields ...

    // ShutdownTimeout is the maximum time to wait for graceful shutdown.
    // After this, loopbacks are force-closed.
    // Default: 30s
    ShutdownTimeout time.Duration

    // DrainTimeout is the maximum time to wait for in-flight messages
    // to drain after external inputs close.
    // Default: ShutdownTimeout
    DrainTimeout time.Duration

    // ShutdownOrder controls how loopbacks are closed.
    // - "simultaneous": Close all at once (default, simplest)
    // - "reverse": Close in reverse registration order
    // - "tracked": Use message tracking for optimal order
    ShutdownOrder string
}
```

## Summary: Complete Solution

| Component | Responsibility |
|-----------|----------------|
| **MessageTracker** | Count in-flight messages, detect drain |
| **Merger** | Track external vs loopback inputs |
| **Distributor** | Track loopback outputs, support early close |
| **ShutdownOrchestrator** | Coordinate phased shutdown |
| **Engine** | Wire everything together |
| **Pipeline Plugin** | Optional: explicit topology declaration |

**Shutdown sequence (complete):**

```
Time    Event
─────   ──────────────────────────────────────────────────
t0      ctx.Cancel() called
t1      Engine stops accepting new external messages
t2      Wait for MessageTracker.Drained() OR timeout
t3      If drained: All messages processed, loopbacks empty
t4      If timeout: Force-close loopbacks
t5      Close loopback outputs (simultaneous or ordered)
t6      Loopback input goroutines detect closed channels
t7      Merger completes (all inputs closed)
t8      Router drains remaining messages
t9      Distributor drains remaining messages
t10     All outputs closed
t11     Engine done channel closes
```

## Conclusion

The recommended solution uses **Engine-coordinated shutdown with Distributor involvement**:

- **No heuristics:** Deterministic based on message tracking
- **Simple API:** Existing plugin API unchanged
- **Clean shutdown:** Loopback outputs closed explicitly, not timed out
- **Distributor involved:** Leverages Distributor's knowledge of outputs
- **Complex pipeline support:** MessageTracker + Pipeline plugin
- **User responsibility:** None—orchestration is automatic

This approach satisfies all user requirements:
1. ✅ Simple API (plugin unchanged)
2. ✅ Involves Distributor
3. ✅ No heuristics (reference counting + done channels)
4. ✅ Clean shutdown
5. ✅ Handles complex multi-loopback pipelines
6. ✅ Optional Pipeline plugin for explicit topology
