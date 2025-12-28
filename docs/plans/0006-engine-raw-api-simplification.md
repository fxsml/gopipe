# Plan 0006: Engine Raw API Simplification

**Status:** Proposed
**Related ADRs:** -
**Depends On:** [Plan 0005](0005-engine-implementation-review.md)

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
├── typedMerger   *pipe.Merger[*Message]
├── distributor   *pipe.Distributor[*Message]
├── router        *Router
├── marshaler     Marshaler
├── errorHandler  ErrorHandler
└── bufferSize    int
```

**Simplified flows:**
```
AddInput:     typed ch → typedMerger
AddRawInput:  raw ch → filter → unmarshal → typedMerger  (convenience)
AddOutput:    distributor → typed ch
AddRawOutput: distributor → marshal → raw ch             (convenience)
AddLoopback:  distributor → typedMerger                  (convenience)
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

**After (~25 lines):**
```go
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    e.mu.Lock()
    if e.started {
        e.mu.Unlock()
        return nil, ErrAlreadyStarted
    }
    e.started = true
    e.mu.Unlock()

    // 1. Start typed merger
    typedMerged, err := e.typedMerger.Merge(ctx)
    if err != nil {
        return nil, err
    }

    // 2. Route messages to handlers
    handled, err := e.router.Pipe(ctx, typedMerged)
    if err != nil {
        return nil, err
    }

    // 3. Start distributor
    distributeDone, err := e.distributor.Distribute(ctx, handled)
    if err != nil {
        return nil, err
    }

    // 4. Wait for completion
    go func() {
        select {
        case <-distributeDone:
        case <-ctx.Done():
        }
        close(e.done)
    }()

    return e.done, nil
}
```

## What Gets Removed

| Component | Lines | Purpose |
|-----------|-------|---------|
| `rawMerger` field | - | Merging raw inputs |
| `rawDistributor` field | - | Distributing raw outputs |
| `typedOutputs` array + struct | ~10 | Storing pre-Start outputs |
| `rawOutputs` array + struct | ~10 | Storing pre-Start raw outputs |
| `hasRawInputs` flag | - | Tracking raw input usage |
| Forwarding goroutines in Start() | ~30 | Forwarding to stored channels |
| Raw output infrastructure in Start() | ~25 | rawDistributor setup |
| Raw input handling in Start() | ~8 | rawMerger → unmarshal |

**Total:** ~80+ lines removed, ~5 lines added

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

- [ ] `AddOutput` adds directly to distributor, no storage
- [ ] `AddRawOutput` is convenience wrapper: `e.marshal(e.AddOutput(...))`
- [ ] `AddRawInput` is convenience wrapper: `e.AddInput(e.unmarshal(...))`
- [ ] `rawMerger` removed from Engine
- [ ] `rawDistributor` removed from Engine
- [ ] `typedOutputs`, `rawOutputs` arrays removed
- [ ] `hasRawInputs` flag removed
- [ ] `Start()` reduced to ~25 lines
- [ ] No forwarding goroutines
- [ ] `make test && make build && make vet` passes
