# PRO-0002: Cancel Path Simplification

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0026

## Overview

Remove the dedicated cancel goroutine from processor, simplifying the cancellation model.

## Goals

1. Remove extra goroutine per processor
2. Simplify error handling via config callback
3. Reduce complexity in processor lifecycle

## Task

**Goal:** Remove dedicated cancel goroutine from processor

**Current:** (processor.go:151-159)
```go
wgCancel := sync.WaitGroup{}
wgCancel.Add(1)
go func() {
    <-ctx.Done()
    for val := range in {
        proc.Cancel(val, newErrCancel(ctx.Err()))
    }
    wgCancel.Done()
}()
```

**Target:**
```go
// Workers just return on context cancellation
select {
case <-ctx.Done():
    return  // No drain
case val, ok := <-in:
    // process...
}

// Error handling via config callback
if config.OnError != nil {
    config.OnError(err)
}
```

**Files to Modify:**
- `processor.go` - Remove cancel goroutine, add OnError callback

**Trade-offs:**
- **Lost:** Draining in-flight items on cancel
- **Gained:** Simpler model, one less goroutine per processor
- **Mitigation:** Use message acknowledgments for reliability

**Acceptance Criteria:**
- [ ] Cancel goroutine removed from processor.go
- [ ] OnError callback in ProcessorConfig
- [ ] All processor tests pass
- [ ] CHANGELOG updated

## Related

- [PRO-0001](../PRO-0001-processor-config/) - ProcessorConfig (implement together)
- [PRO-0026](../../adr/PRO-0026-pipe-processor-simplification.md) - Pipe Simplification ADR
