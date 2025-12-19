# PRO-0009: Non-Generic ProcessorConfig Struct

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0033

## Overview

Define a non-generic `ProcessorConfig` struct that replaces verbose generic functional options.

## Goals

1. Remove generic type parameters from config
2. Enable config reuse across different pipe types
3. Provide sensible defaults

## Task

**Goal:** Create `ProcessorConfig` struct with all settings

**Current:**
```go
pipe := NewProcessPipe(
    handler,
    WithConcurrency[Order, ShippingCommand](4),
    WithBuffer[Order, ShippingCommand](100),
)
```

**Target:**
```go
type ProcessorConfig struct {
    Concurrency    int
    Buffer         int
    Timeout        time.Duration
    CleanupTimeout time.Duration
    CleanupFunc    func(ctx context.Context)
    OnError        func(input any, err error)
}

var DefaultProcessorConfig = ProcessorConfig{
    Concurrency:    1,
    Buffer:         0,
    CleanupTimeout: 30 * time.Second,
}

pipe := NewPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Buffer:      100,
})
```

**Files to Create/Modify:**
- `processor_config.go` (new) - ProcessorConfig struct and defaults
- `processor.go` - Update to use ProcessorConfig
- `pipe.go` - Update constructors

**Acceptance Criteria:**
- [ ] `ProcessorConfig` struct defined
- [ ] `DefaultProcessorConfig` with sensible defaults
- [ ] All settings moved from options to config
- [ ] `OnError` callback uses `any` type
- [ ] Tests pass with new config
- [ ] CHANGELOG updated

## Related

- [PRO-0033](../../adr/PRO-0033-non-generic-processor-config.md) - ADR
- [PRO-0008](../PRO-0008-separate-middleware-from-config/) - Config/Middleware separation
