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

## Conclusion

The recommended solution uses **Engine-coordinated shutdown with Distributor involvement**:

- **No heuristics:** Deterministic based on external input completion
- **Simple API:** Existing plugin API unchanged
- **Clean shutdown:** Loopback outputs closed explicitly, not timed out
- **Distributor involved:** Leverages Distributor's knowledge of outputs

This approach satisfies all user requirements:
1. ✅ Simple API (plugin unchanged)
2. ✅ Involves Distributor
3. ✅ No heuristics (context/done channels)
4. ✅ Clean shutdown
