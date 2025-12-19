# ADR 0033: Non-Generic ProcessorConfig Struct

**Date:** 2025-12-17
**Status:** Proposed
**Supersedes:** Generic functional options for configuration

## Context

Every pipe option currently requires explicit generic parameters:

```go
// Current - verbose and error-prone
pipe := NewProcessPipe(
    handler,
    WithConcurrency[Order, ShippingCommand](4),
    WithBuffer[Order, ShippingCommand](100),
    WithTimeout[Order, ShippingCommand](5*time.Second),
    WithRetryConfig[Order, ShippingCommand](RetryConfig{...}),
    WithRecover[Order, ShippingCommand](),
)
```

This is:
- Repetitive (types specified 5+ times)
- Error-prone (must match exactly)
- Verbose compared to config struct

## Decision

Replace functional options with a non-generic config struct for settings:

```go
type ProcessorConfig struct {
    // Worker pool settings
    Concurrency int           // Default: 1
    Buffer      int           // Output channel buffer, default: 0

    // Timeout settings
    Timeout time.Duration     // Per-item timeout, 0 = no timeout

    // Cleanup settings
    CleanupTimeout time.Duration // Max wait for cleanup, default: 30s
    CleanupFunc    func(ctx context.Context)

    // Error handling
    OnError func(input any, err error) // Called on processing error
}

// Default config
var DefaultProcessorConfig = ProcessorConfig{
    Concurrency:    1,
    Buffer:         0,
    CleanupTimeout: 30 * time.Second,
    CleanupFunc:    func(ctx context.Context) {},
}
```

**Usage:**

```go
pipe := NewPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Buffer:      100,
    Timeout:     5 * time.Second,
    OnError: func(input any, err error) {
        log.Printf("gopipe: error processing %v: %v", input, err)
    },
})
```

**Why `any` for error handlers:**
- Config is non-generic, can be reused
- Handlers receive `any` which they can type-assert if needed
- Aligns with messaging perspective where everything is `*Message`
- Enables config to be defined once, applied to many processors

## Consequences

**Positive:**
- Simpler API with less generic boilerplate
- Config can be reused across different pipe types
- No type parameters to specify for settings
- Easier to document and test

**Negative:**
- Breaking change from current functional options API
- `any` callbacks require type assertions
- Less compile-time type safety in callbacks

**Migration:**

```go
// Old
pipe := NewProcessPipe(
    handler,
    WithConcurrency[In, Out](4),
    WithTimeout[In, Out](5*time.Second),
)

// New
pipe := NewProcessPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Timeout:     5 * time.Second,
})
```

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0032](PRO-0032-spearate-middleware-from-config.md) - Separate Middleware from Config
- Related: [PRO-0037](PRO-0037-middleware-package-consolidation.md) - Middleware Package Consolidation
