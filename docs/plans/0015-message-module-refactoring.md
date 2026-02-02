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

This plan **must** be executed after Plan 0008 (Pipe Interface Refactoring) is complete. The pipe package changes are breaking, and message module code will not compile until updated.

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
| `*pipe.GeneratePipe[*message.RawMessage]` | `*pipe.GeneratorPipe[*message.RawMessage]` |
| `pipe.NewGenerator(...)` | `pipe.NewGenerator(...)` (unchanged) |

Note: Constructor name unchanged, only struct type name changes.

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

## No Changes Required

The following pipe package items are used by message module but **do not change**:

| Item | Notes |
|------|-------|
| `pipe.Distributor` | Infrastructure, not renamed |
| `pipe.Merger` | Infrastructure, not renamed |
| `pipe.Config` | Unchanged |
| `pipe.ErrAlreadyStarted` | Unchanged |
| `pipe.ErrShutdownDropped` | Unchanged |
| `pipe.NewGenerator` | Constructor name unchanged |

## Optional Enhancements

These are **not required** for compatibility but could be considered for future work.

### Enhancement 1: Use Producer Pattern for Subscriber

CloudEvents Subscriber could use the new `Producer` pattern:

```go
// Current
type Subscriber struct {
    gen *pipe.GeneratorPipe[*message.RawMessage]
}

func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *message.RawMessage, error) {
    return s.gen.Generate(ctx)  // Generate() API changes
}

// Optional future enhancement
type Subscriber struct {
    producer pipe.Producer[*message.RawMessage]
}

func NewSubscriber(...) *Subscriber {
    gen := pipe.NewGenerator(s.receive, pipeCfg)
    producer := pipe.NewProducer(trigger.Infinite(), gen)
    return &Subscriber{producer: producer}
}
```

**Decision:** Defer. Current approach works fine, and Producer pattern adds complexity without clear benefit for this use case.

### Enhancement 2: Pre/Post Mapping for Router

Add optional type mapping before/after handler processing (see Plan 0008 Outlook).

**Decision:** Defer to separate feature work if needed.

### Enhancement 3: Expose Semantic Methods

Consider exposing direct semantic methods like `Process()` on Router for testing:

```go
// Optional: Direct invocation for testing
func (r *Router) Process(ctx context.Context, msg *Message) ([]*Message, error) {
    return r.process(ctx, msg)
}
```

**Decision:** Defer. Current testing approach via channels is sufficient.

## Migration Impact

### For Message Module Users

**No breaking changes** to message module's public API:
- `MarshalPipe`, `UnmarshalPipe` - same interface
- `Router` - same interface
- `Publisher`, `Subscriber` - same interface
- `Distributor`, `Merger` - same interface

Internal implementation changes only.

### For Users Extending Message Module

Users who directly use pipe types from message module fields (e.g., accessing `publisher.sink`) will need to update type references:
- `ProcessPipe` → `ProcessorPipe`
- `GeneratePipe` → `GeneratorPipe`

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

## Acceptance Criteria

- [ ] All message module code compiles with new pipe API
- [ ] All message module tests pass
- [ ] No changes to message module public API
- [ ] Build passes (`make build && make vet`)
- [ ] Released simultaneously with pipe refactoring

## Release Notes

When releasing, document:
1. Pipe package breaking changes (see Plan 0008)
2. Message module: internal updates only, no public API changes
3. Migration guide for users extending internal types
