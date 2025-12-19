# ADR 0037: Middleware Package Consolidation

**Date:** 2025-12-17
**Status:** Proposed

## Context

Middleware is currently scattered across the codebase with inconsistent organization. Some middleware lives in the root package (retry, recover), some is internal, and message-specific middleware doesn't have a home.

## Decision

Consolidate all middleware into a `middleware/` package with clear organization:

```
middleware/
├── middleware.go       # Core types and interfaces
├── timeout.go          # Context timeout middleware
├── retry.go            # Retry middleware (moved from root)
├── metrics.go          # Metrics collection middleware
├── logging.go          # Logging middleware
├── recover.go          # Panic recovery middleware
├── tracing.go          # OpenTelemetry tracing middleware
├── message/            # Message-specific middleware
│   ├── correlation.go  # Correlation ID propagation
│   ├── validation.go   # CloudEvents validation
│   └── routing.go      # Error routing middleware
└── doc.go              # Package documentation
```

### Core Middleware Interface

```go
// middleware/middleware.go
package middleware

import "github.com/fxsml/gopipe"

// Middleware wraps a processor with additional behavior
type Middleware[In, Out any] = gopipe.Middleware[In, Out]

// MiddlewareFunc is a function that creates middleware
type MiddlewareFunc[In, Out any] = func(gopipe.Processor[In, Out]) gopipe.Processor[In, Out]

// Chain combines multiple middleware into one
func Chain[In, Out any](middleware ...Middleware[In, Out]) Middleware[In, Out] {
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        for i := len(middleware) - 1; i >= 0; i-- {
            next = middleware[i](next)
        }
        return next
    }
}
```

### Timeout Middleware

```go
// middleware/timeout.go
package middleware

// TimeoutConfig configures timeout behavior
type TimeoutConfig struct {
    Duration         time.Duration // Per-item timeout
    PropagateContext bool          // Propagate parent context, default: true
}

// WithTimeout creates timeout middleware
func WithTimeout[In, Out any](config TimeoutConfig) Middleware[In, Out]
```

### Retry Middleware (Moved from root)

```go
// middleware/retry.go
package middleware

// RetryConfig configures retry behavior (moved from gopipe.RetryConfig)
type RetryConfig struct {
    ShouldRetry ShouldRetryFunc
    Backoff     BackoffFunc
    MaxAttempts int
    Timeout     time.Duration
}

// WithRetry creates retry middleware
func WithRetry[In, Out any](config RetryConfig) Middleware[In, Out]

// Backoff strategies
func ConstantBackoff(delay time.Duration, jitter float64) BackoffFunc
func ExponentialBackoff(initial time.Duration, factor float64, max time.Duration, jitter float64) BackoffFunc
```

### Recovery Middleware

```go
// middleware/recover.go
package middleware

// RecoverConfig configures panic recovery
type RecoverConfig struct {
    OnPanic     func(input any, recovered any, stack []byte)
    ReturnError bool // Return error instead of re-panicking
}

// WithRecover creates panic recovery middleware
func WithRecover[In, Out any](config RecoverConfig) Middleware[In, Out]
```

### Message-Specific Middleware

```go
// middleware/message/correlation.go
package message

// WithCorrelation propagates correlation ID from input to outputs
func WithCorrelation() middleware.Middleware[*message.Message, *message.Message]

// WithTraceContext propagates OpenTelemetry trace context
func WithTraceContext() middleware.Middleware[*message.Message, *message.Message]
```

```go
// middleware/message/validation.go
package message

// WithValidation validates CloudEvents attributes on output messages
func WithValidation(strict bool) middleware.Middleware[*message.Message, *message.Message]
```

### Migration from Root Package

| Old Location | New Location |
|--------------|--------------|
| `gopipe.WithRetryConfig` | `middleware.WithRetry` |
| `gopipe.RetryConfig` | `middleware.RetryConfig` |
| `gopipe.BackoffFunc` | `middleware.BackoffFunc` |
| `gopipe.ShouldRetryFunc` | `middleware.ShouldRetryFunc` |
| `gopipe.useMetrics` (internal) | `middleware.WithMetrics` |
| `gopipe.useRecover` (internal) | `middleware.WithRecover` |
| `gopipe.useContext` (internal) | `middleware.WithTimeout` |

**Backward Compatibility:**

```go
// gopipe/deprecated.go
package gopipe

import "github.com/fxsml/gopipe/middleware"

// Deprecated: Use middleware.WithRetry instead
func WithRetryConfig[In, Out any](config RetryConfig) Option[In, Out] {
    return WithMiddleware(middleware.WithRetry[In, Out](middleware.RetryConfig(config)))
}
```

## Consequences

**Positive:**
- Clear organization of all middleware
- Discoverable API (all in one package)
- Message-specific middleware has a home
- Easier to add new middleware

**Negative:**
- Breaking import paths
- Requires deprecation period for old locations
- More packages to import

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0032](PRO-0032-spearate-middleware-from-config.md) - Separate Middleware from Config
- Related: [PRO-0035](PRO-0035-message-error-routing.md) - Message-Specific Error Routing
- Related: [PRO-0036](PRO-0036-logging-metrics-middleware.md) - Logging and Metrics Middleware
- Related: [IMP-0015](IMP-0015-middleware-pattern.md) - Middleware Pattern
