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

**Fix:** Use `pipe.NewFilterPipe` which supports error handling:
```go
func (e *Engine) applyTypedInputMatcher(in <-chan *Message, matcher Matcher) <-chan *Message {
    if matcher == nil {
        return in
    }
    p := pipe.NewFilterPipe(
        func(ctx context.Context, msg *Message) (bool, error) {
            if matcher.Match(msg) {
                return true, nil
            }
            return false, ErrInputRejected
        },
        pipe.Config{
            ErrorHandler: func(in any, err error) {
                e.errorHandler(in.(*Message), err)
            },
        },
    )
    out, _ := p.Pipe(context.Background(), in)
    return out
}
```

---

### Issue 2: ProcessPipe used where TransformPipe is appropriate

**Location:** `engine.go` - `createMarshalPipe`, `createUnmarshalPipe`

**Problem:**
- These pipes always return exactly 1 output on success
- `ProcessPipe` (1:N) is semantically incorrect for 1:1 transforms

**Fix:** Use `pipe.NewTransformPipe`:
```go
func (e *Engine) createMarshalPipe() *pipe.ProcessPipe[*Message, *RawMessage] {
    return pipe.NewTransformPipe(
        func(ctx context.Context, msg *Message) (*RawMessage, error) {
            data, err := e.marshaler.Marshal(msg.Data)
            if err != nil {
                return nil, err
            }
            if msg.Attributes == nil {
                msg.Attributes = make(Attributes)
            }
            msg.Attributes["datacontenttype"] = e.marshaler.DataContentType()
            return &RawMessage{
                Data:       data,
                Attributes: msg.Attributes,
            }, nil
        },
        pipe.Config{
            ErrorHandler: func(in any, err error) {
                e.errorHandler(in.(*Message), err)
            },
        },
    )
}
```

Same for `createUnmarshalPipe()`.

---

### Issue 3: Manual forwarding goroutines

**Location:** Multiple places in `Start()` and `AddOutput`/`AddRawOutput`

**Problem:** Boilerplate repeated 7+ times:
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

**Fix:** Create helper function:
```go
func forward[T any](ctx context.Context, from <-chan T, to chan T) {
    go func() {
        defer close(to)
        for msg := range from {
            select {
            case to <- msg:
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

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
| `applyTypedInputMatcher` | channel.Process | pipe.NewFilterPipe | Filtering with error handler |
| `applyRawInputMatcher` | channel.Process | pipe.NewFilterPipe | Filtering with error handler |
| `createMarshalPipe` | pipe.NewProcessPipe | pipe.NewTransformPipe | 1:1 transform |
| `createUnmarshalPipe` | pipe.NewProcessPipe | pipe.NewTransformPipe | 1:1 transform |
| `createHandlerPipe` | pipe.NewProcessPipe | pipe.NewProcessPipe | 1:N correct (handler can return multiple) |

---

## Implementation Order

### Phase 1: Quick Wins (Low Risk)
1. Replace `ProcessPipe` with `TransformPipe` for marshal/unmarshal
2. Create forwarding helper function
3. Add BufferSize to EngineConfig

### Phase 2: Filter Refactor (Medium Risk)
4. Replace `channel.Process` with `pipe.NewFilterPipe` for matchers
5. Create helper for raw error handling

---

## Test Plan

1. All existing tests continue to pass after each phase
2. Verify context cancellation behavior unchanged
3. `make test && make build && make vet` passes

---

## Acceptance Criteria

- [ ] No `channel.Process` used for filtering
- [ ] `TransformPipe` used for 1:1 transforms (marshal/unmarshal)
- [ ] Forwarding helper reduces code duplication
- [ ] Buffer sizes configurable via EngineConfig
- [ ] `make test && make build && make vet` passes
