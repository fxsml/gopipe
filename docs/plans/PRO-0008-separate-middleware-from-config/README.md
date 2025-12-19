# PRO-0008: Separate Middleware from Config

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0032

## Overview

Separate configuration concerns (static settings) from behavioral middleware (processor wrappers) in the pipe constructor API.

## Goals

1. Clear distinction between config and middleware
2. Config passed as struct, middleware as variadic
3. Remove mixed-concern options

## Task

**Goal:** Refactor pipe constructors to accept separate config and middleware

**Current:**
```go
// Mixed concerns in single variadic
pipe := NewProcessPipe(
    handler,
    WithConcurrency[In, Out](4),     // Config
    WithTimeout[In, Out](5*time.Second), // Middleware
    WithRetry[In, Out](retryConfig),     // Middleware
)
```

**Target:**
```go
// Separated concerns
func NewPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,                    // Non-generic config
    middleware ...Middleware[In, Out],         // Generic middleware
) Pipe[In, Out]

pipe := NewPipe(
    handler,
    ProcessorConfig{Concurrency: 4},
    WithTimeout[In, Out](5*time.Second),
    WithRetry[In, Out](retryConfig),
)
```

**Files to Create/Modify:**
- `pipe.go` - Update constructors to accept config + middleware separately
- `processor_config.go` - Ensure config only contains settings
- `middleware.go` - Ensure middleware handles behavioral concerns

**Acceptance Criteria:**
- [ ] `NewPipe` accepts `(handler, config, ...middleware)`
- [ ] `NewProcessPipe` updated similarly
- [ ] `NewBatchPipe` updated similarly
- [ ] Config contains only static settings
- [ ] Middleware handles all behavioral wrapping
- [ ] Tests pass with new API
- [ ] CHANGELOG updated

## Related

- [PRO-0032](../../adr/PRO-0032-spearate-middleware-from-config.md) - ADR
- [PRO-0009](../PRO-0009-non-generic-processor-config/) - ProcessorConfig struct
- [PRO-0013](../PRO-0013-middleware-package-consolidation/) - Middleware package
