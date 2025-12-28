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

### Issue 3: Unnecessary forwarding in after-start case

**Location:** `AddOutput`/`AddRawOutput` after-start case

**Problem:** We create an extra channel and forward when we don't need to:
```go
// Current after-start case - WASTEFUL:
msgCh, _ := e.distributor.AddOutput(matcher)  // Distributor creates channel
ch := make(chan *Message, 100)                 // We create ANOTHER channel
go func() { forward msgCh → ch }()             // Pointless forwarding
return ch
```

`distributor.AddOutput()` already returns a usable channel. Why create another and forward?

**Analysis:**

| Case | Forwarding Needed? | Reason |
|------|-------------------|--------|
| AddOutput **after** Start | **No** | Distributor exists, return its channel directly |
| AddOutput **before** Start | Yes | Must return channel before distributor exists |
| Start() wiring | Yes | Channel already returned to user, must connect |
| Loopbacks | Yes | Multiple sources → single merger input |

**Fix for after-start case - eliminate forwarding entirely:**
```go
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message {
    e.mu.Lock()
    defer e.mu.Unlock()

    if e.started {
        // Return distributor's channel directly - no forwarding!
        ch, _ := e.distributor.AddOutput(func(msg *Message) bool {
            return cfg.Matcher == nil || cfg.Matcher.Match(msg)
        })
        return ch
    }

    // Before start - must create channel now, connect later in Start()
    ch := make(chan *Message, 100)
    e.typedOutputs = append(e.typedOutputs, typedOutputEntry{ch: ch, config: cfg})
    return ch
}
```

Same fix applies to `AddRawOutput` after-start case.

**For before-start case:** Forwarding is unavoidable with current API design. Alternative designs would be breaking changes.

**For loopbacks in Start():** Could use `channel.Merge` instead of multiple forwarding goroutines, but current approach works.

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

## Semantic Correctness Table

| Function | Current | Correct | Reason |
|----------|---------|---------|--------|
| `applyTypedInputMatcher` | channel.Process | channel.Filter | Filtering with error side effect |
| `applyRawInputMatcher` | channel.Process | channel.Filter | Filtering with error side effect |
| `createMarshalPipe` | pipe.ProcessPipe | channel.Process | 1:0/1 mapping, no pipe needed |
| `createUnmarshalPipe` | pipe.ProcessPipe | channel.Process | 1:0/1 mapping, no pipe needed |
| `createHandlerPipe` | pipe.ProcessPipe | pipe.ProcessPipe | 1:N correct (handler returns multiple) |
| AddOutput after-start | create ch + forward | return distributor ch | No forwarding needed |
| AddRawOutput after-start | create ch + forward | return distributor ch | No forwarding needed |

---

## Implementation Order

### Phase 1: Quick Wins (Low Risk)
1. Replace `channel.Process` with `channel.Filter` for matchers
2. Eliminate forwarding in AddOutput/AddRawOutput after-start case
3. Add BufferSize to EngineConfig
4. Create helper for raw error handling

### Phase 2: Simplify Marshal/Unmarshal
5. Replace `pipe.ProcessPipe` with `channel.Process` for marshal/unmarshal

---

## Test Plan

1. All existing tests continue to pass after each phase
2. Verify context cancellation behavior unchanged
3. `make test && make build && make vet` passes

---

## Acceptance Criteria

- [ ] `channel.Filter` used for input matchers
- [ ] `channel.Process` used for marshal/unmarshal
- [ ] No forwarding goroutines in AddOutput/AddRawOutput after-start case
- [ ] Buffer sizes configurable via EngineConfig
- [ ] Error helper reduces code duplication
- [ ] `make test && make build && make vet` passes
