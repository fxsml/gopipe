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
