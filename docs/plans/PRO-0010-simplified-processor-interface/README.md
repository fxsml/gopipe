# PRO-0010: Simplified Processor Interface

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0034

## Overview

Simplify the Processor interface by removing the Cancel method. Error handling moves to config callbacks and middleware.

## Goals

1. Remove Cancel method from Processor interface
2. Simplify processor implementations
3. Move error handling to ProcessorConfig.OnError

## Task

**Goal:** Remove Cancel method, use OnError callback

**Current:**
```go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) (Out, error)
    Cancel(In, error)  // Remove this
}
```

**Target:**
```go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

func (f ProcessFunc[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
    return f(ctx, in)
}
```

**Migration Path:**
```go
// Old Cancel logic
func (p *MyProcessor) Cancel(in Order, err error) {
    log.Printf("canceled: %v", err)
}

// Moves to config
config := ProcessorConfig{
    OnError: func(in any, err error) {
        log.Printf("error: %v", err)
    },
}
```

**Files to Create/Modify:**
- `processor.go` - Remove Cancel from interface
- `process_func.go` - Update ProcessFunc
- `pipe.go` - Remove cancel path goroutine
- All existing processors - Remove Cancel implementations

**Acceptance Criteria:**
- [ ] Cancel method removed from Processor interface
- [ ] ProcessFunc updated to single-method implementation
- [ ] Cancel goroutine removed from pipe internals
- [ ] All built-in processors updated
- [ ] Tests pass without Cancel
- [ ] Migration guide documented
- [ ] CHANGELOG updated

## Related

- [PRO-0034](../../adr/PRO-0034-simplified-processor-interface.md) - ADR
- [PRO-0002](../PRO-0002-cancel-path/) - Cancel path refactoring
