# Plan 0015: Message Module Refactoring

**Status:** Proposed
**Blocked By:** [0014 - Pipe Interface Refactoring](./0014-pipe-interface-refactoring.md)

## Overview

Update the message module to use the new pipe package API after the refactoring in Plan 0014. This plan separates **essential** changes (required for compatibility) from **optional** enhancements.

## Goals

1. Update message module to work with refactored pipe package
2. Maintain API compatibility for message module consumers where possible
3. Release simultaneously with pipe package changes

## Dependencies

This plan **must** be executed after Plan 0014 (Pipe Interface Refactoring) is complete. The pipe package changes are breaking, and message module code will not compile until updated.

## Essential Updates

These changes are **required** for the message module to compile after pipe refactoring.

### Task 1: Update MarshalPipe

**File:** `message/pipes.go`

| Before | After |
|--------|-------|
| `*pipe.ProcessPipe[*RawMessage, *Message]` | `*pipe.ProcessorPipe[*RawMessage, *Message]` |
| `pipe.NewProcessPipe(...)` | `pipe.NewProcessor(...)` |

### Task 2: Update UnmarshalPipe

**File:** `message/pipes.go`

| Before | After |
|--------|-------|
| `*pipe.ProcessPipe[*Message, *RawMessage]` | `*pipe.ProcessorPipe[*Message, *RawMessage]` |
| `pipe.NewProcessPipe(...)` | `pipe.NewProcessor(...)` |

### Task 3: Update Router

**File:** `message/router.go`

| Before | After |
|--------|-------|
| `pipe.NewProcessPipe(fn, cfg)` | `pipe.NewProcessor(fn, cfg)` |

Router uses `pipe.ProcessPipe` internally but doesn't expose it. Only constructor call changes.

### Task 4: Update CloudEvents Publisher

**File:** `message/cloudevents/publisher.go`

| Before | After |
|--------|-------|
| `*pipe.ProcessPipe[*message.RawMessage, struct{}]` | `*pipe.SinkPipe[*message.RawMessage]` |
| `pipe.NewSinkPipe(...)` | `pipe.NewSink(...)` |

### Task 5: Update CloudEvents Subscriber

**File:** `message/cloudevents/subscriber.go`

| Before | After |
|--------|-------|
| `*pipe.GeneratePipe[*message.RawMessage]` | `*pipe.SourcePipe[*message.RawMessage]` |
| `pipe.NewGenerator(...)` | `pipe.NewSource(...)` |

Both struct type and constructor name change.

### Task 6: Update Middleware Type References

**Files:** Various test files and any middleware usage

Check for references to old middleware types if pipe middleware signatures changed.

### Task 7: Update Tests

**Files:**
- `message/pipes_test.go`
- `message/router_test.go`
- `message/cloudevents/publisher_test.go`
- `message/cloudevents/subscriber_test.go`

Update any test code that references old type names.

### Task 8: Add Router.Route() Semantic Method

**File:** `message/router.go`

Add `Route()` method to expose the semantic interface, consistent with pipe package pattern:

```go
// Route processes a single message through the router (semantic interface).
// Does NOT apply middleware - calls handler directly.
// Use Pipe() for streaming with middleware.
func (r *Router) Route(ctx context.Context, msg *Message) ([]*Message, error) {
    return r.process(ctx, msg)
}
```

This provides:
- Consistent pattern with `Processor.Process()`, `Source.Source()`, etc.
- Direct invocation for testing without channel setup
- Clear semantic: Router routes messages

## No Changes Required

The following pipe package items are used by message module but **do not change**:

| Item | Notes |
|------|-------|
| `pipe.Distributor` | Infrastructure, not renamed |
| `pipe.Merger` | Infrastructure, not renamed |
| `pipe.Config` | Unchanged |
| `pipe.ErrAlreadyStarted` | Unchanged |
| `pipe.ErrShutdownDropped` | Unchanged |

## Optional Enhancements

These are **not required** for compatibility but could be considered for future work.

### Enhancement 1: Use Producer Pattern for Subscriber

CloudEvents Subscriber could use the new `Producer` pattern:

```go
// Current
type Subscriber struct {
    src *pipe.SourcePipe[*message.RawMessage]
}

func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
    return s.src.Pipe(ctx, triggerChan)  // needs external trigger
}

// Optional future enhancement
type Subscriber struct {
    producer pipe.Producer[*message.RawMessage]
}

func NewSubscriber(...) *Subscriber {
    src := pipe.NewSource(s.receive, pipeCfg)
    producer := pipe.NewProducer(trigger.Infinite(), src)
    return &Subscriber{producer: producer}
}
```

**Decision:** Defer. Current approach works fine, and Producer pattern adds complexity without clear benefit for this use case.

### Enhancement 2: Pre/Post Mapping for Router

Add optional type mapping before/after handler processing (see Plan 0014 Outlook).

**Decision:** Defer to separate feature work if needed.

### Enhancement 3: Additional Semantic Methods

Other message module types could expose semantic methods for consistency:

```go
// MarshalPipe could have:
func (p *MarshalPipe) Marshal(ctx, *RawMessage) ([]*Message, error)

// UnmarshalPipe could have:
func (p *UnmarshalPipe) Unmarshal(ctx, *Message) ([]*RawMessage, error)
```

**Decision:** Defer. Router.Route() is included as essential (Task 8). Other types can be added later if needed.

## Migration Impact

### For Message Module Users

**No breaking changes** to message module's public API:
- `MarshalPipe`, `UnmarshalPipe` - same interface
- `Router` - same interface, plus new `Route()` method
- `Publisher`, `Subscriber` - same interface
- `Distributor`, `Merger` - same interface

**New API addition:**
- `Router.Route(ctx, *Message) ([]*Message, error)` - direct invocation for testing

### For Users Extending Message Module

Users who directly use pipe types from message module fields (e.g., accessing `publisher.sink`) will need to update type references:
- `ProcessPipe` → `ProcessorPipe`
- `GeneratePipe` → `SourcePipe`

This is rare and considered acceptable breakage.

## Tasks Summary

| # | Task | File(s) | Essential |
|---|------|---------|-----------|
| 1 | Update MarshalPipe | `message/pipes.go` | Yes |
| 2 | Update UnmarshalPipe | `message/pipes.go` | Yes |
| 3 | Update Router | `message/router.go` | Yes |
| 4 | Update CloudEvents Publisher | `message/cloudevents/publisher.go` | Yes |
| 5 | Update CloudEvents Subscriber | `message/cloudevents/subscriber.go` | Yes |
| 6 | Update Middleware References | Various | Yes |
| 7 | Update Tests | `*_test.go` | Yes |
| 8 | Add Router.Route() | `message/router.go` | Yes |

## Acceptance Criteria

- [ ] All message module code compiles with new pipe API
- [ ] All message module tests pass
- [ ] Router.Route() method added and tested
- [ ] Build passes (`make build && make vet`)
- [ ] Released simultaneously with pipe refactoring

## Release Notes

When releasing, document:
1. Pipe package breaking changes (see Plan 0014)
2. Message module: internal updates + new `Router.Route()` method
3. Migration guide for users extending internal types
