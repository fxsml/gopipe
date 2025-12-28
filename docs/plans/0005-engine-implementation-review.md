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

### Issue 3: Manual forwarding goroutines

**Location:** Multiple places in `Start()` and `AddOutput`/`AddRawOutput`

**Problem:** Boilerplate repeated for forwarding with context cancellation:
```go
go func(out chan *Message, msgCh <-chan *Message) {
    for msg := range msgCh {
        select {
        case out <- msg:
        case <-ctx.Done():
            return
        }
    }
    close(out)
}(output.ch, msgCh)
```

**Analysis:**

This pattern exists because:
1. `AddOutput` creates output channel before `Start` (returns channel to caller)
2. `Start` creates data source (distributor subscriber)
3. Need to bridge them

**Existing utilities that could help:**

1. **Loopback channels**: Use `channel.Merge` instead of multiple forwarding goroutines
   ```go
   loopbackChannels := make([]<-chan *Message, 0, len(e.loopbacks))
   for _, loopback := range e.loopbacks {
       ch := e.typedDistributor.Subscribe(ctx)
       ch = channel.Filter(ch, loopback.config.Matcher.Match)
       loopbackChannels = append(loopbackChannels, ch)
   }
   loopbackMerged := channel.Merge(loopbackChannels...)
   e.typedMerger.Add(loopbackMerged)
   ```

2. **Context cancellation**: Use `channel.Cancel` for context-aware forwarding
   ```go
   msgCh = channel.Cancel(ctx, msgCh, func(msg *Message, err error) {
       // Optional: handle in-flight messages on cancellation
   })
   ```

3. **Output forwarding**: The forwarding from subscriber to pre-created output.ch is inherent to the API design. Options:
   - Keep forwarding (simplest)
   - Change API so AddOutput returns channel created during Start (breaking change)
   - Use a channel wrapper/proxy pattern (adds complexity)

**Recommendation:** Use `channel.Merge` for loopbacks, `channel.Cancel` where context awareness is needed, accept forwarding for outputs as inherent to API design.

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
| loopback forwarding | manual goroutines | channel.Merge | Multiple sources → single channel |

---

## Implementation Order

### Phase 1: Quick Wins (Low Risk)
1. Replace `channel.Process` with `channel.Filter` for matchers
2. Add BufferSize to EngineConfig
3. Create helper for raw error handling

### Phase 2: Simplify Marshal/Unmarshal
4. Replace `pipe.ProcessPipe` with `channel.Process` for marshal/unmarshal

### Phase 3: Loopback Refactor
5. Replace loopback forwarding goroutines with `channel.Merge`

---

## Test Plan

1. All existing tests continue to pass after each phase
2. Verify context cancellation behavior unchanged
3. `make test && make build && make vet` passes

---

## Acceptance Criteria

- [ ] `channel.Filter` used for input matchers (simpler than pipe)
- [ ] `channel.Process` used for marshal/unmarshal (simpler than pipe)
- [ ] `channel.Merge` used for loopback (eliminates forwarding goroutines)
- [ ] Buffer sizes configurable via EngineConfig
- [ ] Error helper reduces code duplication
- [ ] `make test && make build && make vet` passes
