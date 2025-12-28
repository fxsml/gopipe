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

### Issue 6: Loopbacks create N goroutines instead of using channel.Merge

**Location:** `Start()` lines 328-345

**Current:**
```go
for _, lb := range e.loopbacks {
    loopbackCh, _ := e.distributor.AddOutput(lbMatcher)
    go func(ch <-chan *Message) {
        for msg := range ch {
            loopbackIn <- msg  // N goroutines all writing to same channel
        }
    }(loopbackCh)
}
```

**Problem:** N loopbacks = N distributor outputs + N forwarding goroutines, all feeding ONE loopbackIn channel. This is just merging!

**Fix using channel.Merge:**
```go
var loopbackChannels []<-chan *Message
for _, lb := range e.loopbacks {
    ch, _ := e.distributor.AddOutput(lb.config.Matcher.Match)
    loopbackChannels = append(loopbackChannels, ch)
}
if len(loopbackChannels) > 0 {
    merged := channel.Merge(loopbackChannels...)
    e.typedMerger.AddInput(merged)
}
```

**Even simpler - ONE distributor output:**
Since all loopbacks feed into the same place, and distributor is first-match-wins:
```go
if len(e.loopbacks) > 0 {
    // Combine all loopback matchers with Any
    matchers := make([]Matcher, len(e.loopbacks))
    for i, lb := range e.loopbacks {
        matchers[i] = lb.config.Matcher
    }
    combined := match.Any(matchers...)

    loopbackCh, _ := e.distributor.AddOutput(combined.Match)
    e.typedMerger.AddInput(loopbackCh)  // Direct! No forwarding!
}
```

---

### Issue 7: Matcher interface should use Attributes, not Message

**Location:** `matcher.go`, `match/*.go`, all matcher call sites in `engine.go`

**Current:**
```go
type Matcher interface {
    Match(msg *Message) bool
}
```

**Problem:**
- All existing matchers (Types, Sources, All, Any) only access `msg.Attributes`, never `msg.Data`
- Raw message matching requires creating garbage: `&Message{Attributes: msg.Attributes}`
- Interface is dishonest about what matchers actually need

**Fix:** Change Matcher to use Attributes directly:
```go
type Matcher interface {
    Match(attrs Attributes) bool
}
```

**Benefits:**
1. Eliminates allocation in raw distributor matching
2. Honest API - reflects what matchers actually need
3. Works for both Message and RawMessage (both have Attributes)
4. Simpler caller code: `matcher.Match(msg.Attributes)` vs wrapper creation

**Migration:**
1. Update `Matcher` interface in `matcher.go`
2. Update `types.go`, `sources.go` signatures
3. Update `match.go` (All/Any) signatures
4. Update all call sites in `engine.go`
5. Update tests

---

### Issue 8: Handler lookup happens twice for raw messages

**Location:** `createUnmarshalPipe()` line 469, `createHandlerPipe()` line 500

**Current:**
```go
// In unmarshal:
entry, ok := e.handlers[ceType]  // Lookup 1
instance := entry.handler.NewInput()

// In handler pipe (same message, later):
entry, ok := e.handlers[ceType]  // Lookup 2 (redundant)
entry.handler.Handle(ctx, msg)
```

**Fix:** Store handler reference in message context or a field:
```go
// In unmarshal:
entry, ok := e.handlers[ceType]
msg.handler = entry.handler  // Cache it

// In handler pipe:
msg.handler.Handle(ctx, msg)  // No lookup
```

Minor optimization - only matters for high throughput.

---

### Issue 9: pipe.Apply could compose pipelines

**Available but unused:**
```go
pipeline := pipe.Apply(pipeA, pipeB)  // Composes two pipes
```

**Current architecture prevents this** because merger sits between unmarshal and handler. But with simplified architecture, could potentially use.

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
| N loopback outputs + goroutines | 1 output with match.Any | All feed same merger |
| `Matcher.Match(*Message)` | `Matcher.Match(Attributes)` | All matchers only use attrs |
| Double handler lookup | Cache handler in message | Minor optimization |

---

## Implementation Order

### Phase 1: Simplify Engine Structure (High Impact)
1. Create distributor and mergers in `NewEngine()`
2. `AddOutput()`/`AddInput()` call components directly
3. Remove `typedOutputs`, `rawOutputs`, `typedInputs`, `rawInputs` storage
4. Simplify `Start()` to just wire and start components

### Phase 2: Simplify Loopbacks
5. Combine loopback matchers with `match.Any`
6. Single distributor output for all loopbacks
7. Direct feed to typedMerger (no forwarding)

### Phase 3: Simplify Primitives
8. Replace `channel.Process` with `channel.Filter` for matchers
9. Replace `pipe.ProcessPipe` with `channel.Process` for marshal/unmarshal

### Phase 4: Configuration & Cleanup
10. Add BufferSize to EngineConfig
11. Create helper for raw error handling
12. Change `Matcher.Match(*Message)` to `Matcher.Match(Attributes)`

---

## Test Plan

1. All existing tests continue to pass after each phase
2. Verify context cancellation behavior unchanged
3. `make test && make build && make vet` passes

---

## Acceptance Criteria

- [ ] Distributor and mergers created in `NewEngine()`
- [ ] `AddOutput()`/`AddInput()` call components directly, no storage
- [ ] No forwarding goroutines for outputs
- [ ] Loopbacks use single distributor output with combined matcher
- [ ] `channel.Filter` used for input matchers
- [ ] `channel.Process` used for marshal/unmarshal
- [ ] Buffer sizes configurable via EngineConfig
- [ ] `Matcher.Match(Attributes)` - interface uses Attributes directly
- [ ] `make test && make build && make vet` passes
