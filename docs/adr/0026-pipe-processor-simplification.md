# ADR 0026: Pipe and Processor Simplification

**Date:** 2025-12-13
**Status:** Proposed
**Supersedes:** Partial aspects of current implementation

## Context

Before implementing the CloudEvents standardization (ADRs 0019-0025), we need to review and simplify the core pipe and processor abstractions. The current implementation has several areas that could be improved:

### Current State Analysis

#### 1. Generic Verbosity in Options

Every option requires explicit generic parameters:

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

#### 2. Cancel Path Complexity

The processor has a dedicated goroutine for draining on cancellation:

```go
// Current cancel path in StartProcessor
go func() {
    <-ctx.Done()
    for val := range in {
        proc.Cancel(val, newErrCancel(ctx.Err()))
    }
    wgCancel.Done()
}()
```

**Problems:**
- Extra goroutine per processor
- Complex synchronization (wgProcess + wgCancel)
- Cancel vs Failure distinction adds cognitive load
- Most use cases just want to log and continue

#### 3. Mixed Concerns in Options

Options mix configuration (settings) with behavior (middleware):

| Current Option | Is It Config or Middleware? |
|----------------|----------------------------|
| `WithConcurrency` | Config (worker count) |
| `WithBuffer` | Config (channel size) |
| `WithTimeout` | Middleware (wraps processor) |
| `WithRetry` | Middleware (wraps processor) |
| `WithRecover` | Middleware (wraps processor) |
| `WithCancel` | Mixed (adds cancel handlers) |
| `WithMiddleware` | Middleware (explicit) |
| `WithMetrics` | Middleware (wraps processor) |

This mixing makes it unclear what happens when and in what order.

#### 4. FanIn Initialization Quirk

```go
func (f *FanIn[T]) isStarted() bool {
    return f.inputs == nil  // Uses nil as flag - non-obvious
}
```

#### 5. BatchPipe Generic Parameter Issue

```go
func NewBatchPipe[In any, Out any](
    ...,
    opts ...Option[[]In, Out],  // Odd: []In not In
) Pipe[In, Out]
```

## Decision

### 1. Non-Generic Config Struct

Replace functional options with a config struct for settings:

```go
// New - clean and simple
type ProcessorConfig struct {
    // Worker pool settings
    Concurrency int           // Default: 1
    Buffer      int           // Output channel buffer, default: 0

    // Timeout settings
    Timeout            time.Duration // Per-item timeout, 0 = no timeout
    ContextPropagation bool          // Default: true

    // Cleanup settings
    CleanupTimeout time.Duration // Max wait for cleanup, default: 30s

    // Error handling
    OnError     func(input any, err error) // Called on processing error
    OnPanic     func(input any, recovered any, stack string) // Called on panic

    // Recovery
    RecoverPanics bool // Default: false
}

// Default config
var DefaultProcessorConfig = ProcessorConfig{
    Concurrency:        1,
    Buffer:             0,
    ContextPropagation: true,
    CleanupTimeout:     30 * time.Second,
    RecoverPanics:      false,
}
```

**Usage:**
```go
pipe := NewProcessPipe(handler, ProcessorConfig{
    Concurrency:   4,
    Buffer:        100,
    Timeout:       5 * time.Second,
    RecoverPanics: true,
    OnError: func(input any, err error) {
        log.Printf("error processing %v: %v", input, err)
    },
})
```

**Why `any` for error handlers:**
- Config is non-generic, can be reused
- Handlers receive `any` which they can type-assert if needed
- Aligns with messaging perspective where everything is `*Message`
- Enables middleware to be configured once, applied to many processors

### 2. Simplified Cancel Path

Remove the dedicated cancel goroutine. Instead:

```go
// Simplified StartProcessor
func StartProcessor[In, Out any](
    ctx context.Context,
    in <-chan In,
    proc Processor[In, Out],
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) <-chan Out {
    out := make(chan Out, config.Buffer)

    go func() {
        defer close(out)
        var wg sync.WaitGroup

        for i := 0; i < config.Concurrency; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                for {
                    select {
                    case <-ctx.Done():
                        // Just return - don't drain
                        return
                    case val, ok := <-in:
                        if !ok {
                            return
                        }
                        results, err := proc.Process(ctx, val)
                        if err != nil {
                            // Call error handler if configured
                            if config.OnError != nil {
                                config.OnError(val, err)
                            }
                            continue
                        }
                        for _, r := range results {
                            select {
                            case out <- r:
                            case <-ctx.Done():
                                return
                            }
                        }
                    }
                }
            }()
        }

        wg.Wait()
        runCleanup(ctx, config)
    }()

    return out
}
```

**Benefits:**
- Simpler mental model
- No separate Cancel method on Processor
- Error handling via config callback
- On cancellation, workers just stop (no drain)

**Trade-off:**
- In-flight items may be lost on cancellation
- For message systems: use acknowledgments for reliability instead

### 3. Clear Separation: Config vs Middleware

| Concern | Config Struct | Middleware |
|---------|---------------|------------|
| Worker count | `Concurrency` | - |
| Channel buffer | `Buffer` | - |
| Cleanup timeout | `CleanupTimeout` | - |
| Context propagation | `ContextPropagation` | - |
| Error callback | `OnError` | - |
| Panic callback | `OnPanic` | - |
| Panic recovery | `RecoverPanics` | - |
| **Timeout** | - | `WithTimeout(d)` |
| **Retry** | - | `WithRetry(cfg)` |
| **Metrics** | - | `WithMetrics(collector)` |
| **Logging** | - | `WithLogging(logger)` |
| **Tracing** | - | `WithTracing(tracer)` |
| **Custom** | - | User-provided |

**Rule of thumb:**
- **Config**: Static settings that don't wrap the processor
- **Middleware**: Behavior that wraps Process() calls

### 4. Middleware as Separate Variadic

```go
// Clear separation
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,                    // Non-generic config
    middleware ...Middleware[In, Out],         // Generic middleware
) Pipe[In, Out]

// Middleware definition (unchanged)
type Middleware[In, Out any] func(Processor[In, Out]) Processor[In, Out]
```

**Usage:**
```go
pipe := NewProcessPipe(
    handler,
    ProcessorConfig{
        Concurrency: 4,
        Timeout:     5 * time.Second,
    },
    WithRetry[Order, ShippingCommand](retryConfig),
    WithMetrics[Order, ShippingCommand](metricsCollector),
)
```

**Why keep middleware generic:**
- Middleware wraps the processor, needs type information
- Enables type-safe transformation in middleware
- Only specified once per middleware, not per-option

### 5. Simplified Processor Interface

```go
// Simplified - no Cancel method
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

// ProcessFunc implements Processor
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

func (f ProcessFunc[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
    return f(ctx, in)
}
```

### 6. Message-Specific Error Routing

For messaging systems, provide error routing middleware:

```go
// WithErrorRouting sends errors to a destination
func WithErrorRouting[In, Out any](
    errorDest string,  // e.g., "gopipe://errors" or "kafka://dlq"
    includeInput bool, // Include original input in error message
) Middleware[*Message, *Message]

// Usage
loop.Route("orders", handler,
    ProcessorConfig{Concurrency: 4},
    WithErrorRouting[*Message, *Message]("gopipe://errors", true),
)
```

### 7. Logging and Metrics Middleware

```go
// Logging middleware
type Logger interface {
    Debug(msg string, fields ...any)
    Info(msg string, fields ...any)
    Warn(msg string, fields ...any)
    Error(msg string, fields ...any)
}

func WithLogging[In, Out any](logger Logger, level LogLevel) Middleware[In, Out]

// Metrics middleware
type MetricsCollector interface {
    RecordProcessed(duration time.Duration, success bool)
    RecordInFlight(delta int)
}

func WithMetrics[In, Out any](collector MetricsCollector) Middleware[In, Out]
```

### 8. Middleware Package Consolidation

Consolidate all middleware into the `middleware/` package with clear organization:

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

#### Core Middleware Interface

```go
// middleware/middleware.go
package middleware

import (
    "github.com/fxsml/gopipe"
)

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

#### Timeout Middleware (New)

```go
// middleware/timeout.go
package middleware

import (
    "context"
    "time"

    "github.com/fxsml/gopipe"
)

// TimeoutConfig configures timeout behavior
type TimeoutConfig struct {
    Duration           time.Duration // Per-item timeout
    PropagateContext   bool          // Propagate parent context, default: true
}

// WithTimeout creates timeout middleware
func WithTimeout[In, Out any](config TimeoutConfig) Middleware[In, Out] {
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        return gopipe.ProcessFunc[In, Out](func(ctx context.Context, in In) ([]Out, error) {
            if config.Duration > 0 {
                var cancel context.CancelFunc
                ctx, cancel = context.WithTimeout(ctx, config.Duration)
                defer cancel()
            }
            if !config.PropagateContext {
                ctx = context.Background()
                if config.Duration > 0 {
                    var cancel context.CancelFunc
                    ctx, cancel = context.WithTimeout(ctx, config.Duration)
                    defer cancel()
                }
            }
            return next.Process(ctx, in)
        })
    }
}
```

#### Retry Middleware (Moved from root)

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

#### Metrics Middleware

```go
// middleware/metrics.go
package middleware

import (
    "time"
)

// MetricsCollector receives processing metrics
type MetricsCollector interface {
    // RecordProcessed records a processing attempt
    RecordProcessed(duration time.Duration, success bool, labels map[string]string)
    // RecordInFlight tracks concurrent processing
    RecordInFlight(delta int, labels map[string]string)
}

// MetricsConfig configures metrics collection
type MetricsConfig struct {
    Collector    MetricsCollector
    LabelFunc    func(in any) map[string]string // Extract labels from input
    RecordInput  bool                           // Include input in labels
}

// WithMetrics creates metrics middleware
func WithMetrics[In, Out any](config MetricsConfig) Middleware[In, Out]

// PrometheusCollector implements MetricsCollector for Prometheus
type PrometheusCollector struct {
    ProcessedTotal   *prometheus.CounterVec
    ProcessedLatency *prometheus.HistogramVec
    InFlight         *prometheus.GaugeVec
}

func NewPrometheusCollector(namespace, subsystem string) *PrometheusCollector
```

#### Logging Middleware

```go
// middleware/logging.go
package middleware

// LogLevel defines logging verbosity
type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelWarn
    LogLevelError
)

// Logger interface for pluggable logging
type Logger interface {
    Log(level LogLevel, msg string, fields map[string]any)
}

// LoggingConfig configures logging behavior
type LoggingConfig struct {
    Logger       Logger
    Level        LogLevel
    LogInput     bool // Log input values (may be sensitive)
    LogOutput    bool // Log output values
    LogDuration  bool // Log processing duration
}

// WithLogging creates logging middleware
func WithLogging[In, Out any](config LoggingConfig) Middleware[In, Out]

// Adapters for popular loggers
func SlogAdapter(logger *slog.Logger) Logger
func ZapAdapter(logger *zap.Logger) Logger
```

#### Recovery Middleware

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

#### Message-Specific Middleware

```go
// middleware/message/correlation.go
package message

import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/middleware"
)

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

```go
// middleware/message/routing.go
package message

// ErrorRoutingConfig configures error message routing
type ErrorRoutingConfig struct {
    Destination    string // e.g., "gopipe://errors", "kafka://dlq"
    IncludeInput   bool   // Include original message in error
    IncludeStack   bool   // Include stack trace
}

// WithErrorRouting routes errors to a destination
func WithErrorRouting(config ErrorRoutingConfig) middleware.Middleware[*message.Message, *message.Message]
```

#### Migration from Root Package

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

## Rationale

### Why Non-Generic Config?

1. **Reusability**: Same config can be used for different pipe types
2. **Simplicity**: No type parameters to specify
3. **Messaging alignment**: In message systems, everything is `*Message`
4. **Callback flexibility**: `any` callbacks work with type assertions

### Why Remove Cancel Path?

1. **Simplicity**: Removes goroutine and synchronization complexity
2. **Reliability model**: Use message acknowledgments instead
3. **Error handling**: `OnError` callback is sufficient
4. **Performance**: One less goroutine per processor

### Why Separate Config and Middleware?

1. **Clarity**: Clear what's a setting vs behavior
2. **Order independence**: Config applied once, middleware composes
3. **Testing**: Config can be tested without middleware
4. **Documentation**: Easier to explain

## Consequences

### Positive

- Simpler API with less generic boilerplate
- Clearer mental model (config vs middleware)
- Better messaging alignment
- Easier testing and documentation

### Negative

- Breaking change from current API
- `any` callbacks require type assertions
- Loss of dedicated cancel draining (mitigated by acks)

### Migration

```go
// Old
pipe := NewProcessPipe(
    handler,
    WithConcurrency[In, Out](4),
    WithTimeout[In, Out](5*time.Second),
    WithCancel[In, Out](func(in In, err error) {
        log.Printf("canceled: %v", err)
    }),
)

// New
pipe := NewProcessPipe(
    handler,
    ProcessorConfig{
        Concurrency: 4,
        Timeout:     5 * time.Second,
        OnError: func(in any, err error) {
            log.Printf("error: %v", err)
        },
    },
)
```

## Links

- [ADR 0013: Processor Abstraction](0013-processor-abstraction.md)
- [ADR 0014: Composable Pipe](0014-composable-pipe.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
