# Plan 0006: Engine Raw API Simplification

**Status:** Complete
**Related ADRs:** -
**Depends On:** [Plan 0005](0005-engine-implementation-review.md)
**Design Evolution:** [0006-config-convention.decisions.md](0006-config-convention.decisions.md)

## Overview

Simplify Engine by making raw input/output methods convenience wrappers around typed methods. This removes the need for separate raw merger/distributor and eliminates storage arrays and forwarding goroutines.

## Current State (After Plan 0005)

```
Engine struct:
├── typedMerger      *pipe.Merger[*Message]
├── rawMerger        *pipe.Merger[*RawMessage]     ← REMOVE
├── distributor      *pipe.Distributor[*Message]
├── rawDistributor   *pipe.Distributor[*RawMessage] ← REMOVE
├── typedOutputs     []typedOutputEntry             ← REMOVE
├── rawOutputs       []rawOutputEntry               ← REMOVE
├── hasRawInputs     bool                           ← REMOVE
└── router           *Router
```

**Current flows:**
```
AddRawInput:  raw ch → rawMerger → (Start) unmarshal → typedMerger
AddRawOutput: (Start) distributor catch-all → marshal → rawDistributor → raw ch
```

## Proposed Simplification

### AddRawOutput as convenience wrapper

**Before (3 code paths, ~25 lines):**
```go
func (e *Engine) AddRawOutput(cfg RawOutputConfig) <-chan *RawMessage {
    e.mu.Lock()
    defer e.mu.Unlock()

    if !e.started {
        ch := make(chan *RawMessage, e.bufferSize)
        e.rawOutputs = append(e.rawOutputs, rawOutputEntry{ch: ch, config: cfg})
        return ch
    }

    if e.rawDistributor != nil {
        ch, _ := e.rawDistributor.AddOutput(...)
        return ch
    }

    // Fallback: per-output marshal
    msgCh, _ := e.distributor.AddOutput(...)
    return e.marshal(msgCh)
}
```

**After (1 line):**
```go
func (e *Engine) AddRawOutput(cfg RawOutputConfig) <-chan *RawMessage {
    return e.marshal(e.AddOutput(OutputConfig{Matcher: cfg.Matcher}))
}
```

### AddRawInput as convenience wrapper

**Before (~10 lines + Start() handling):**
```go
func (e *Engine) AddRawInput(ch <-chan *RawMessage, cfg RawInputConfig) error {
    e.mu.Lock()
    e.hasRawInputs = true
    e.mu.Unlock()

    filtered := e.applyRawInputMatcher(ch, cfg.Matcher)
    _, err := e.rawMerger.AddInput(filtered)
    return err
}

// In Start():
if e.hasRawInputs {
    rawMerged, _ := e.rawMerger.Merge(ctx)
    e.typedMerger.AddInput(e.unmarshal(rawMerged))
}
```

**After (2 lines):**
```go
func (e *Engine) AddRawInput(ch <-chan *RawMessage, cfg RawInputConfig) error {
    filtered := e.applyRawInputMatcher(ch, cfg.Matcher)
    return e.AddInput(e.unmarshal(filtered), InputConfig{})
}
```

### AddOutput simplified (no storage)

**Before:**
```go
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
    e.mu.Lock()
    defer e.mu.Unlock()

    if !e.started {
        ch := make(chan *Message, e.bufferSize)
        e.typedOutputs = append(e.typedOutputs, typedOutputEntry{ch: ch, config: cfg})
        return ch
    }

    outMatcher := cfg.Matcher
    ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
        return outMatcher == nil || outMatcher.Match(msg.Attributes)
    })
    return ch
}
```

**After:**
```go
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
    outMatcher := cfg.Matcher
    ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
        return outMatcher == nil || outMatcher.Match(msg.Attributes)
    })
    return ch
}
```

## Resulting Engine Structure

```
Engine struct:
├── merger        *pipe.Merger[*Message]      // renamed from typedMerger
├── distributor   *pipe.Distributor[*Message]
├── router        *Router
├── marshaler     Marshaler
├── errorHandler  ErrorHandler
└── bufferSize    int
```

Note: `typedMerger` renamed to `merger` since there's only one merger after simplification.
Also: `e.done` field removed - return distributor's done channel directly (see below).

**Simplified flows:**
```
AddInput:     typed ch → merger
AddRawInput:  raw ch → filter → unmarshal → merger  (convenience)
AddOutput:    distributor → typed ch
AddRawOutput: distributor → marshal → raw ch        (convenience)
AddLoopback:  distributor → merger                  (convenience)
```

## Start() Simplification

**Before (~80 lines):**
```go
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    // ... started check ...

    // 1. Set up raw input path if any raw inputs were added
    if e.hasRawInputs {
        rawMerged, _ := e.rawMerger.Merge(ctx)
        e.typedMerger.AddInput(e.unmarshal(rawMerged))
    }

    // 2. Start typed merger
    typedMerged, _ := e.typedMerger.Merge(ctx)

    // 3. Route messages to handlers
    handled, _ := e.router.Pipe(ctx, typedMerged)

    // 4. Add typed outputs (forwarding goroutines)
    for _, output := range e.typedOutputs {
        // ... 15 lines of forwarding ...
    }

    // 5. Set up raw output infrastructure
    if len(e.rawOutputs) > 0 {
        // ... 25 lines of rawDistributor setup ...
    }

    // 6. Start distributor
    distributeDone, _ := e.distributor.Distribute(ctx, handled)

    // 7. Wait for completion
    // ...
}
```

**After (~15 lines):**
```go
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    e.mu.Lock()
    if e.started {
        e.mu.Unlock()
        return nil, ErrAlreadyStarted
    }
    e.started = true
    e.mu.Unlock()

    // 1. Start merger
    merged, err := e.merger.Merge(ctx)
    if err != nil {
        return nil, err
    }

    // 2. Route messages to handlers
    handled, err := e.router.Pipe(ctx, merged)
    if err != nil {
        return nil, err
    }

    // 3. Start distributor and return its done channel directly
    return e.distributor.Distribute(ctx, handled)
}
```

**Why return distributor's done channel directly:**
- The upstream pipeline (merger, router) closes channels on context cancellation
- This propagates to distributor's input, closing its done channel
- The wrapper goroutine was redundant - distributor already handles this
- Removes need for `e.done` field entirely

## What Gets Removed

| Component | Lines | Purpose |
|-----------|-------|---------|
| `rawMerger` field | - | Merging raw inputs |
| `rawDistributor` field | - | Distributing raw outputs |
| `typedOutputs` array + struct | ~10 | Storing pre-Start outputs |
| `rawOutputs` array + struct | ~10 | Storing pre-Start raw outputs |
| `hasRawInputs` flag | - | Tracking raw input usage |
| `done` channel field | - | Wrapper for distributor's done |
| `ctx` field | - | Stored context (no longer needed) |
| Forwarding goroutines in Start() | ~30 | Forwarding to stored channels |
| Raw output infrastructure in Start() | ~25 | rawDistributor setup |
| Raw input handling in Start() | ~8 | rawMerger → unmarshal |
| Done wrapper goroutine in Start() | ~7 | Wrapper for ctx.Done() |

**Total:** ~90+ lines removed, ~5 lines added

## What Gets Renamed

| Before | After | Reason |
|--------|-------|--------|
| `typedMerger` | `merger` | Only one merger after simplification |

## Trade-offs

### Per-output marshal vs shared marshal

**Before:** All raw outputs share one marshal goroutine
**After:** Each raw output has its own marshal goroutine

Impact: Minimal. Marshal is CPU-bound (JSON encoding), parallelization may actually improve throughput.

### Per-input unmarshal vs shared unmarshal

**Before:** All raw inputs share one unmarshal goroutine (after rawMerger)
**After:** Each raw input has its own unmarshal goroutine

Impact: Positive. Parallelizes unmarshal work across inputs.

## Implementation Order

1. Simplify `AddOutput` - remove storage, add directly to distributor
2. Simplify `AddRawOutput` - make convenience wrapper
3. Simplify `AddRawInput` - make convenience wrapper
4. Remove `rawMerger`, `rawDistributor`, storage arrays from Engine struct
5. Simplify `Start()` - remove all output/input wiring code
6. Update tests if needed

## Test Plan

1. All existing tests continue to pass
2. Verify ordering behavior (user controls via call order)
3. `make test && make build && make vet` passes

## Acceptance Criteria

- [x] `AddOutput` adds directly to distributor, no storage
- [x] `AddRawOutput` is convenience wrapper: `e.marshal(e.AddOutput(...))`
- [x] `AddRawInput` is convenience wrapper: `e.AddInput(e.unmarshal(...))`
- [x] `rawMerger` removed from Engine
- [x] `rawDistributor` removed from Engine
- [x] `typedOutputs`, `rawOutputs` arrays removed
- [x] `hasRawInputs` flag removed
- [x] `done` channel field removed - return distributor's done directly
- [x] `ctx` field removed
- [x] `typedMerger` renamed to `merger`
- [x] `Start()` reduced to ~15 lines
- [x] No forwarding goroutines
- [x] `make test && make build && make vet` passes

## Implementation Notes

- Added `ShutdownTimeout: 100ms` to merger config to ensure clean shutdown on context cancellation
- Without ShutdownTimeout, merger waits indefinitely for input channels to close
- Net reduction: 175 lines removed, 26 added (-149 lines)
