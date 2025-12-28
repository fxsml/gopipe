# Plan 0005: Engine Implementation Fixes

**Status:** Proposed
**Related ADRs:** -
**Depends On:** [Plan 0001](0001-message-engine.md)

## Overview

Fix implementation issues in the current Engine: correct use of channel/pipe primitives, reduce boilerplate, and add missing configuration options.

## Issues and Fixes

### Issue 1: channel.Process misused for filtering

**Location:** `engine.go` - `applyTypedInputMatcher`, `applyRawInputMatcher`

**Problem:**
```go
return channel.Process(in, func(msg *Message) []*Message {
    if matcher.Match(msg) {
        return []*Message{msg}
    }
    e.errorHandler(msg, ErrInputRejected)
    return nil
})
```

- `channel.Process` is for 1:N mapping (one input → zero or more outputs)
- This is filtering with side effects (error handler call)
- Semantically incorrect

**Fix:** Use `channel.Filter` with direct error handler call:
```go
func (e *Engine) applyTypedInputMatcher(in <-chan *Message, matcher Matcher) <-chan *Message {
    if matcher == nil {
        return in
    }
    return channel.Filter(in, func(msg *Message) bool {
        if matcher.Match(msg) {
            return true
        }
        e.errorHandler(msg, ErrInputRejected)
        return false
    })
}
```

This is simpler than pipe.NewFilterPipe - no context, no pipe config, just a filter with side effects.

---

### Issue 2: ProcessPipe used where simpler approach works

**Location:** `engine.go` - `createMarshalPipe`, `createUnmarshalPipe`

**Problem:**
- These use `ProcessPipe` which requires context and pipe configuration
- The pipe package adds complexity not needed here

**Fix:** Use `channel.Process` with direct error handler call:
```go
func (e *Engine) createMarshalFunc() func(<-chan *Message) <-chan *RawMessage {
    return func(in <-chan *Message) <-chan *RawMessage {
        return channel.Process(in, func(msg *Message) []*RawMessage {
            data, err := e.marshaler.Marshal(msg.Data)
            if err != nil {
                e.errorHandler(msg, err)
                return nil
            }
            if msg.Attributes == nil {
                msg.Attributes = make(Attributes)
            }
            msg.Attributes["datacontenttype"] = e.marshaler.DataContentType()
            return []*RawMessage{{
                Data:       data,
                Attributes: msg.Attributes,
            }}
        })
    }
}
```

Same for unmarshal. Returns `nil` on error (0 outputs), `[]*T{result}` on success (1 output).

---

### Issue 3: Unnecessary forwarding - create components in NewEngine()

**Location:** All of `AddOutput`, `AddRawOutput`, `AddInput`, `AddRawInput`, `Start()`

**Problem:** Current design creates intermediate channels and forwards:
```go
// Current flow - OVERCOMPLICATED:
NewEngine()     → (nothing created)
AddOutput()     → create ch, store config
Start()         → create distributor, call distributor.AddOutput(), forward to stored ch
```

**Key insight:** `Distributor.AddOutput()` and `Merger.AddInput()` work **before** `Distribute()`/`Merge()` is called. They just store the entry and return.

**Simpler design - create components in NewEngine():**
```go
// Simplified flow:
NewEngine()     → create distributor, create mergers
AddOutput()     → call distributor.AddOutput(), return its channel directly
AddInput()      → call merger.AddInput() directly
Start()         → call merger.Merge(), distributor.Distribute()
```

**No intermediate channels. No forwarding. No before/after complexity.**

**Implementation:**
```go
func NewEngine(cfg EngineConfig) *Engine {
    e := &Engine{
        marshaler:    cfg.Marshaler,
        errorHandler: cfg.ErrorHandler,
    }

    // Create distributor upfront
    e.distributor = pipe.NewDistributor[*Message](pipe.DistributorConfig[*Message]{
        Buffer: 100,
        NoMatchHandler: func(msg *Message) {
            e.errorHandler(msg, ErrNoMatchingOutput)
        },
    })

    // Create mergers upfront
    e.typedMerger = pipe.NewMerger[*Message](pipe.MergerConfig{Buffer: 100})
    e.rawMerger = pipe.NewMerger[*RawMessage](pipe.MergerConfig{Buffer: 100})

    return e
}

func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
    ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
        return cfg.Matcher == nil || cfg.Matcher.Match(msg)
    })
    return ch
}

func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error {
    filtered := e.applyTypedInputMatcher(ch, cfg.Matcher)
    _, err := e.typedMerger.AddInput(filtered)
    return err
}
```

**Benefits:**
- Eliminates `typedOutputs`, `rawOutputs`, `typedInputs`, `rawInputs` storage
- Eliminates all forwarding goroutines
- Eliminates before/after Start() complexity
- `Start()` just wires the pipeline and calls `Merge()`/`Distribute()`

**For rawDistributor:** Create lazily on first `AddRawOutput()` call, or always create in `NewEngine()`.

---

### Issue 4: Hardcoded buffer sizes

**Problem:** Buffer size 100 hardcoded throughout.

**Fix:** Add to EngineConfig:
```go
type EngineConfig struct {
    Marshaler    Marshaler
    ErrorHandler ErrorHandler
    BufferSize   int  // default 100
}
```

---

### Issue 5: Repeated error helper pattern

**Problem:** `&Message{Attributes: raw.Attributes}` repeated for error handling.

**Fix:** Create helper:
```go
func (e *Engine) handleRawError(raw *RawMessage, err error) {
    e.errorHandler(&Message{Attributes: raw.Attributes}, err)
}
```

---

## Summary Table

| Current | Simplified | Reason |
|---------|------------|--------|
| `channel.Process` for matchers | `channel.Filter` | Filtering with error side effect |
| `pipe.ProcessPipe` for marshal | `channel.Process` | 1:0/1 mapping, no pipe config needed |
| Create distributor in Start() | Create in NewEngine() | AddOutput() works before Distribute() |
| Create mergers in Start() | Create in NewEngine() | AddInput() works before Merge() |
| Store output/input configs | Direct calls to components | No storage needed |
| Forwarding goroutines | None | Return component channels directly |
| Before/after Start() logic | None | Always same path |

---

## Implementation Order

### Phase 1: Simplify Engine Structure (High Impact)
1. Create distributor and mergers in `NewEngine()`
2. `AddOutput()`/`AddInput()` call components directly
3. Remove `typedOutputs`, `rawOutputs`, `typedInputs`, `rawInputs` storage
4. Simplify `Start()` to just wire and start components

### Phase 2: Simplify Primitives
5. Replace `channel.Process` with `channel.Filter` for matchers
6. Replace `pipe.ProcessPipe` with `channel.Process` for marshal/unmarshal

### Phase 3: Configuration
7. Add BufferSize to EngineConfig
8. Create helper for raw error handling

---

## Test Plan

1. All existing tests continue to pass after each phase
2. Verify context cancellation behavior unchanged
3. `make test && make build && make vet` passes

---

## Acceptance Criteria

- [ ] Distributor and mergers created in `NewEngine()`
- [ ] `AddOutput()`/`AddInput()` call components directly, no storage
- [ ] No forwarding goroutines
- [ ] `channel.Filter` used for input matchers
- [ ] `channel.Process` used for marshal/unmarshal
- [ ] Buffer sizes configurable via EngineConfig
- [ ] `make test && make build && make vet` passes
