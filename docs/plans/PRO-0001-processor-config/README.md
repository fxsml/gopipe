# PRO-0001: ProcessorConfig

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0026

## Overview

Replace generic functional options `Option[In, Out]` with a simple non-generic `ProcessorConfig` struct.

## Goals

1. Remove generic type parameters from configuration
2. Simplify processor construction API
3. Provide backward-compatible wrappers

## Task

**Goal:** Replace `Option[In, Out]` with non-generic `ProcessorConfig`

**Current:**
```go
pipe := NewProcessPipe(
    handler,
    WithConcurrency[Order, ShippingCommand](4),
    WithBuffer[Order, ShippingCommand](100),
    WithTimeout[Order, ShippingCommand](5*time.Second),
)
```

**Target:**
```go
type ProcessorConfig struct {
    Concurrency int
    Buffer      int
    Timeout     time.Duration
    OnError     func(error)
}

pipe := NewPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Buffer:      100,
    Timeout:     5 * time.Second,
})
```

**Files to Create/Modify:**
- `processor_config.go` (new) - ProcessorConfig struct
- `processor.go` (modify) - Accept config instead of variadic options
- `pipe.go` (modify) - Add new pipe constructor
- `deprecated.go` (new) - Backward-compat wrappers for old options

**Acceptance Criteria:**
- [ ] `ProcessorConfig` struct defined with all settings
- [ ] `StartProcessor` accepts config instead of variadic options
- [ ] All pipe constructors updated
- [ ] Deprecated wrappers for old `With*` options
- [ ] Tests pass with new API
- [ ] CHANGELOG updated

## Related

- [PRO-0026](../../adr/PRO-0026-pipe-processor-simplification.md) - Pipe Simplification ADR
- [PRO-0002](../PRO-0002-cancel-path/) - Cancel Path (can be done together)
