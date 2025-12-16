# Middleware Package Refactoring Analysis

**Date:** 2025-12-16
**Status:** Analysis
**Related:** Task 0.3 in layer-0-foundation-cleanup.md

## Executive Summary

This document analyzes the current middleware architecture and recommends a consolidation strategy. The goal is to move all middleware to a dedicated `middleware/` package with clear organization, consistent patterns, and separation from configuration.

## Current State

### Middleware-Like Components Scattered Across Root Package

| File | Component | Type | Purpose |
|------|-----------|------|---------|
| `middleware.go` | `MiddlewareFunc[In, Out]` | Type | Core middleware type |
| `retry.go` | `WithRetryConfig` | Option | Retry with backoff |
| `recover.go` | `WithRecover` | Option | Panic recovery |
| `context.go` | `WithTimeout` | Option | Context timeout |
| `context.go` | `WithoutContextPropagation` | Option | Context isolation |
| `metrics.go` | `WithMetricsCollector` | Option | Metrics collection |
| `metadata.go` | `WithMetadataProvider` | Option | Metadata enrichment |
| `log.go` | `WithLogConfig` | Option | Logging configuration |

### Existing `middleware/` Package

| File | Component | Purpose |
|------|-----------|---------|
| `message.go` | `NewMessageMiddleware` | Helper to create message middleware |
| `correlation.go` | `MessageCorrelation` | Propagates correlation ID |

### Current Application Order (option.go:41-82)

```go
func (c *config[In, Out]) apply(proc Processor[In, Out]) Processor[In, Out] {
    // 1. Cancel handlers (wrap Cancel method)
    if c.cancel != nil { ... }

    // 2. Context/Timeout middleware
    if c.timeout > 0 || !c.contextPropagation { ... }

    // 3. Logger → MetricsCollector (always on by default!)
    if c.logConfig == nil { c.logConfig = &defaultLogConfig }
    if logger := newMetricsLogger(*c.logConfig); logger != nil { ... }

    // 4. MetricsCollector middleware
    if len(c.metricsCollector) > 1 { ... }

    // 5. Retry middleware
    if c.retry != nil { ... }

    // 6. User middleware (WithMiddleware)
    proc = applyMiddleware(proc, c.middleware...)

    // 7. Metadata providers
    proc = applyMiddleware(proc, c.metadataProvider...)

    // 8. Recover middleware (outermost)
    if c.recover { ... }

    return proc
}
```

### Issues

1. **Scattered location**: Middleware split between root and `middleware/`
2. **Mixed concerns**: Options mix config settings and middleware
3. **Implicit behavior**: Logging always enabled by default
4. **Complex ordering**: Application order hidden in `apply()` method
5. **Generic verbosity**: Every option requires `[In, Out]` type parameters
6. **MetricsCollector pattern**: Logger uses collector pattern, not middleware
7. **Drop handling**: No middleware access to drop/cancel path (proposed in Task 0.2)

## Proposed Architecture

### Package Structure

```
middleware/
├── doc.go              # Package documentation
├── middleware.go       # Core types, Chain(), Apply()
├── timeout.go          # Context timeout
├── retry.go            # Retry with backoff
├── recover.go          # Panic recovery
├── metrics.go          # Metrics collection
├── logging.go          # Structured logging
├── drop.go             # Drop/cancel handling
├── message/            # Message-specific middleware
│   ├── correlation.go  # Correlation ID propagation
│   ├── validation.go   # CloudEvents validation (future)
│   └── tracing.go      # Distributed tracing (future)
└── internal/           # Internal helpers
    └── backoff.go      # Backoff implementations
```

### Core Types

```go
// middleware/middleware.go
package middleware

import (
    "github.com/fxsml/gopipe"
)

// Middleware wraps a Processor with additional behavior.
type Middleware[In, Out any] = gopipe.MiddlewareFunc[In, Out]

// Chain combines multiple middleware into one.
// Execution order: first middleware is outermost (runs first on entry, last on exit).
func Chain[In, Out any](mw ...Middleware[In, Out]) Middleware[In, Out] {
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        for i := len(mw) - 1; i >= 0; i-- {
            next = mw[i](next)
        }
        return next
    }
}

// Apply applies middleware to a processor (convenience function).
func Apply[In, Out any](proc gopipe.Processor[In, Out], mw ...Middleware[In, Out]) gopipe.Processor[In, Out] {
    return Chain(mw...)(proc)
}
```

### Type-Agnostic Middleware Chains (RECOMMENDED)

**Problem:** Typed middleware chains (`Chain[Order, Cmd](...)`) can only be used with one pipe type. Also, storing configs requires a switch statement that must know all middleware types.

**Solution:** Use a shared `WrapperFunc` signature that all middleware implement. This enables:
1. Type-agnostic chain definition
2. Custom middleware support (no switch statement needed)

```go
// WrapperFunc is the SHARED signature for all middleware.
// It wraps process and drop functions (type-erased).
type WrapperFunc func(
    process func(context.Context, any) ([]any, error),
    drop func(any, error),
) (
    wrappedProcess func(context.Context, any) ([]any, error),
    wrappedDrop func(any, error),
)

// MiddlewareBuilder holds a WrapperFunc for deferred typed instantiation.
type MiddlewareBuilder struct {
    wrap WrapperFunc
}

// Chain combines builders (no type params!)
func Chain(builders ...MiddlewareBuilder) MiddlewareBuilder

// Apply instantiates with specific types
func Apply[In, Out any](b MiddlewareBuilder) Middleware[In, Out]
```

**Middleware returns MiddlewareBuilder, not Middleware[In, Out]:**

```go
// NO type parameters - returns a builder
func Retry(cfg RetryConfig) MiddlewareBuilder {
    return MiddlewareBuilder{
        wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (...) {
            return func(ctx context.Context, in any) ([]any, error) {
                // Retry logic here - works with any types
                return process(ctx, in)
            }, drop
        },
    }
}
```

**Custom middleware uses the SAME pattern:**

```go
// Custom middleware - exact same API as standard middleware
func CircuitBreaker(cfg CircuitBreakerConfig) MiddlewareBuilder {
    return MiddlewareBuilder{
        wrap: func(process func(context.Context, any) ([]any, error), drop func(any, error)) (...) {
            // Custom logic here
            return wrappedProcess, drop
        },
    }
}
```

**Usage:**

```go
// Define ONCE - no type parameters, includes CUSTOM middleware!
productionChain := Chain(
    Recover(RecoverConfig{...}),
    Logging(LoggingConfig{OnDrop: true}),
    Metrics(MetricsConfig{Recorder: recorder}),
    CircuitBreaker(CircuitBreakerConfig{Threshold: 3}),  // Custom!
    Retry(RetryConfig{MaxAttempts: 3}),
)

// Apply to ANY pipe type
orderPipe := NewProcessPipe(orderHandler, nil, config,
    Apply[Order, ShippingCmd](productionChain))

paymentPipe := NewProcessPipe(paymentHandler, nil, config,
    Apply[Payment, Receipt](productionChain))  // Same chain!
```

**Runnable example:** [main_builder.go](main_builder.go)

**Key Benefits:**
1. **Type-agnostic** - Chain defined once, used with any types
2. **Extensible** - Custom middleware uses same API
3. **No switch statements** - No need to enumerate all middleware types
4. **Shared signature** - All middleware implement `WrapperFunc`

### Middleware Implementations

#### Timeout Middleware

```go
// middleware/timeout.go
package middleware

import (
    "context"
    "time"
    "github.com/fxsml/gopipe"
)

// TimeoutConfig configures timeout behavior.
type TimeoutConfig struct {
    Duration         time.Duration // Per-item timeout
    PropagateContext bool          // Default: true
}

// Timeout creates timeout middleware.
func Timeout[In, Out any](cfg TimeoutConfig) Middleware[In, Out] {
    if cfg.Duration <= 0 {
        return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
            return next
        }
    }
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        return gopipe.NewProcessor(
            func(ctx context.Context, in In) ([]Out, error) {
                if !cfg.PropagateContext {
                    ctx = context.Background()
                }
                ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
                defer cancel()
                return next.Process(ctx, in)
            },
            next.Drop,  // or next.Cancel with current naming
        )
    }
}
```

#### Retry Middleware

```go
// middleware/retry.go
package middleware

import (
    "time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
    ShouldRetry func(error) bool
    Backoff     func(attempt int) time.Duration
    MaxAttempts int
    Timeout     time.Duration
}

// DefaultRetryConfig provides sensible defaults.
var DefaultRetryConfig = RetryConfig{
    ShouldRetry: func(error) bool { return true },
    Backoff:     ConstantBackoff(time.Second, 0.2),
    MaxAttempts: 3,
    Timeout:     time.Minute,
}

// Retry creates retry middleware.
func Retry[In, Out any](cfg RetryConfig) Middleware[In, Out] {
    // Implementation moved from gopipe/retry.go
}

// Backoff functions
func ConstantBackoff(delay time.Duration, jitter float64) func(int) time.Duration
func ExponentialBackoff(initial time.Duration, factor float64, max time.Duration, jitter float64) func(int) time.Duration
```

#### Logging Middleware

```go
// middleware/logging.go
package middleware

import (
    "log/slog"
    "time"
)

// Logger interface for pluggable logging.
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}

// LoggingConfig configures logging behavior.
type LoggingConfig struct {
    Logger      Logger        // nil = slog.Default()
    LogSuccess  bool          // Log successful processing
    LogDuration bool          // Include duration in logs
    OnDrop      bool          // Log dropped items (cancel/error)
}

// Logging creates logging middleware.
func Logging[In, Out any](cfg LoggingConfig) Middleware[In, Out]

// Convenience: SlogAdapter wraps *slog.Logger
func SlogAdapter(l *slog.Logger) Logger
```

#### Metrics Middleware

```go
// middleware/metrics.go
package middleware

import (
    "time"
)

// MetricsRecorder receives processing metrics.
type MetricsRecorder interface {
    RecordProcessed(duration time.Duration, success bool, labels map[string]string)
    RecordInFlight(delta int, labels map[string]string)
    RecordDropped(isCancel bool, labels map[string]string)
}

// MetricsConfig configures metrics collection.
type MetricsConfig struct {
    Recorder   MetricsRecorder
    Labels     func(input any) map[string]string  // Extract labels from input
}

// Metrics creates metrics middleware.
func Metrics[In, Out any](cfg MetricsConfig) Middleware[In, Out]
```

#### Drop Handler Middleware

```go
// middleware/drop.go
package middleware

import (
    "errors"
    "github.com/fxsml/gopipe"
)

// DropConfig configures drop handling.
type DropConfig struct {
    OnDrop func(input any, err error, isCancel bool)
}

// Drop creates drop handler middleware.
// Wraps the Drop/Cancel method to intercept dropped items.
func Drop[In, Out any](cfg DropConfig) Middleware[In, Out] {
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        return gopipe.NewProcessor(
            next.Process,
            func(in In, err error) {
                if cfg.OnDrop != nil {
                    isCancel := errors.Is(err, gopipe.ErrCanceled)
                    cfg.OnDrop(in, err, isCancel)
                }
                next.Drop(in, err)
            },
        )
    }
}
```

### Integration with ProcessorConfig

```go
// gopipe/processor_config.go
type ProcessorConfig struct {
    Concurrency int
    Buffer      int
    OnDrop      func(input any, err error)  // Fallback drop handler (from Task 0.2)
}

// gopipe/pipe.go - Updated constructor
func NewProcessPipe[In, Out any](
    process func(context.Context, In) ([]Out, error),
    drop func(In, error),
    config ProcessorConfig,
    mw ...middleware.Middleware[In, Out],  // Explicit middleware
) Pipe[In, Out]
```

### Usage Examples

```go
// Example 1: Basic pipeline with middleware
pipe := gopipe.NewProcessPipe(
    orderHandler,
    nil,  // Use config.OnDrop
    gopipe.ProcessorConfig{Concurrency: 4},
    middleware.Timeout[Order, ShippingCmd](middleware.TimeoutConfig{
        Duration: 5 * time.Second,
    }),
    middleware.Retry[Order, ShippingCmd](middleware.RetryConfig{
        MaxAttempts: 3,
        Backoff:     middleware.ExponentialBackoff(100*time.Millisecond, 2, 5*time.Second, 0.2),
    }),
    middleware.Logging[Order, ShippingCmd](middleware.LoggingConfig{
        Logger:     slog.Default(),
        LogSuccess: true,
        OnDrop:     true,
    }),
)

// Example 2: Chained middleware for reuse
productionMiddleware := middleware.Chain(
    middleware.Recover[*Message, *Message](middleware.RecoverConfig{
        LogPanic: true,
    }),
    middleware.Logging[*Message, *Message](middleware.LoggingConfig{
        Logger: slog.Default(),
    }),
    middleware.Metrics[*Message, *Message](middleware.MetricsConfig{
        Recorder: prometheusRecorder,
    }),
)

// Apply to multiple pipes
orderPipe := gopipe.NewProcessPipe(orderHandler, nil, config, productionMiddleware)
paymentPipe := gopipe.NewProcessPipe(paymentHandler, nil, config, productionMiddleware)
```

## Migration Strategy

### Phase 1: Move Existing Middleware

| Current | New Location | Notes |
|---------|--------------|-------|
| `gopipe.useRetry` | `middleware.Retry` | Public |
| `gopipe.useRecover` | `middleware.Recover` | Public |
| `gopipe.useContext` | `middleware.Timeout` | Public |
| `gopipe.useMetrics` | `middleware.Metrics` | Public |
| `gopipe.useMetadata` | `middleware.Metadata` | Public |

### Phase 2: Deprecate Root Options

```go
// gopipe/deprecated.go

// Deprecated: Use middleware.Retry instead.
func WithRetryConfig[In, Out any](cfg RetryConfig) Option[In, Out] {
    return WithMiddleware(middleware.Retry[In, Out](middleware.RetryConfig(cfg)))
}

// Deprecated: Use middleware.Recover instead.
func WithRecover[In, Out any]() Option[In, Out] {
    return WithMiddleware(middleware.Recover[In, Out](middleware.RecoverConfig{}))
}

// Deprecated: Use middleware.Timeout instead.
func WithTimeout[In, Out any](d time.Duration) Option[In, Out] {
    return WithMiddleware(middleware.Timeout[In, Out](middleware.TimeoutConfig{Duration: d}))
}
```

### Phase 3: Remove Implicit Logging

Current behavior: Logging is always on unless `LogConfig.Disabled = true`.

New behavior: Logging is opt-in via middleware.

```go
// To restore old behavior, users add:
middleware.Logging[In, Out](middleware.LoggingConfig{
    LogSuccess: true,
    OnDrop:     true,
})
```

## Recommendation

### Option A: Full Migration (Recommended)

Move all middleware to `middleware/` package with:
1. Clear, explicit middleware composition
2. No implicit behavior (opt-in logging)
3. Generic middleware chain for reuse
4. Consistent config struct pattern

**Pros:**
- Clean separation of concerns
- Explicit middleware ordering
- Reusable middleware chains
- No surprises (no implicit logging)

**Cons:**
- Breaking change (deprecation period needed)
- More verbose for simple cases

### Option B: Partial Migration

Keep some middleware as Options, move only specialized ones.

**Pros:**
- Less disruptive
- Simpler for basic use cases

**Cons:**
- Mixed patterns persist
- Unclear what's an Option vs Middleware

### Option C: Keep Current + Add middleware/ Enhancements

Don't migrate, just enhance `middleware/` package.

**Pros:**
- No breaking changes

**Cons:**
- Technical debt persists
- Confusing dual patterns

## Final Recommendation: Option A

**Rationale:**
1. Aligns with ProcessorConfig refactoring (Task 0.1)
2. Aligns with drop path refactoring (Task 0.2)
3. Makes middleware explicit and composable
4. Follows Go idiom of explicit over implicit
5. Since we're at v0.x.x, breaking changes are acceptable

**Implementation Order:**
1. Create new middleware types in `middleware/`
2. Migrate implementations
3. Add deprecated wrappers in root
4. Update documentation
5. Remove implicit logging behavior

## Related Documents

- [Layer 0: Foundation Cleanup](../layer-0-foundation-cleanup.md)
- [Drop Path Refactoring](../cancel-path-refactoring/)
- [ADR 0026: Pipe and Processor Simplification](../../adr/0026-pipe-processor-simplification.md)
