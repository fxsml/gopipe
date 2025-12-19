# ADR 0032: Separate Middleware from Config

**Date:** 2025-12-17
**Status:** Proposed
**Supersedes:** Mixed concerns in current functional options

## Context

Current options mix configuration (settings) with behavior (middleware):

| Current Option | Is It Config or Middleware? |
|----------------|----------------------------|
| `WithConcurrency` | Config (worker count) |
| `WithBuffer` | Config (channel size) |
| `WithTimeout` | Middleware (context timeout) |
| `WithRetry` | Middleware (wraps processor) |
| `WithRecover` | Middleware (panic recovery) |
| `WithCancel` | Config (default OnError) |
| `WithMiddleware` | Middleware (explicit) |
| `WithMetrics` | Middleware (wraps processor) |
| `WithContext` | Middleware (context propagation) |

This mixing makes it unclear what happens when and in what order.

## Decision

### Clear Separation: Config vs Middleware

| Concern | Config Struct | Middleware |
|---------|---------------|------------|
| Worker count | `Concurrency` | - |
| Channel buffer | `Buffer` | - |
| Cleanup timeout | `CleanupTimeout` | - |
| Error callback | `OnError` | - |
| **Context** | - | `WithContext(cfg)` |
| **Timeout** | - | `WithContext(cfg)` |
| **Recover** | - | `WithRecover(cfg)` |
| **Retry** | - | `WithRetry(cfg)` |
| **Metrics** | - | `WithMetrics(collector)` |
| **Logging** | - | `WithLogging(logger)` |
| **Tracing** | - | `WithTracing(tracer)` |
| **Custom** | - | User-provided |

**Note:** Context propagation and timeout are merged into a single `WithContext` middleware.

**Rule of thumb:**
- **Config**: Static settings that don't wrap the processor
- **Middleware**: Behavior that wraps Process() calls

### Middleware as Separate Variadic

```go
// Clear separation
func NewPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,                    // Non-generic config
    middleware ...Middleware[In, Out],         // Generic middleware
) Pipe[In, Out]

// Middleware definition (unchanged)
type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]
```

**Usage:**

```go
pipe := NewPipe(
    handler,
    ProcessorConfig{
        Concurrency: 4,
    },
    WithContext[Order, ShippingCommand](ContextConfig{
        Timeout:     5 * time.Second,
        Propagate:   true,
    }),
    WithRecover[Order, ShippingCommand](RecoverConfig{
        OnPanic: func(in any, r any) { log.Printf("panic: %v", r) },
    }),
    WithRetry[Order, ShippingCommand](retryConfig),
    WithMetrics[Order, ShippingCommand](metricsCollector),
)
```

**Why keep middleware generic:**
- Middleware wraps the processor, needs type information
- Enables type-safe transformation in middleware
- Only specified once per middleware, not per-option

## Consequences

**Positive:**
- Clear mental model (config vs middleware)
- Order independence for config, explicit order for middleware
- Easier to test config without middleware
- Better documentation clarity

**Negative:**
- Breaking change from current mixed options API
- Two different APIs to learn (config struct + middleware variadic)

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0033](PRO-0033-non-generic-processor-config.md) - Non-Generic ProcessorConfig
- Related: [PRO-0037](PRO-0037-middleware-package-consolidation.md) - Middleware Package Consolidation
