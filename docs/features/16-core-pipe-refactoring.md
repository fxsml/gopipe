# Feature 16: Core Pipe Refactoring

**Package:** `gopipe`
**Status:** Proposed
**Priority:** High (prerequisite for CloudEvents standardization)

**Related ADRs:**
- [ADR 0026](../adr/0026-pipe-processor-simplification.md) - Pipe and Processor Simplification
- [ADR 0027](../adr/0027-fan-out-pattern.md) - Fan-Out Pattern
- [ADR 0028](../adr/0028-generator-source-patterns.md) - Generator and Source Patterns

## Summary

Before implementing CloudEvents standardization (ADRs 0019-0025), the core pipe and processor abstractions need simplification. This feature consolidates three architectural changes:

1. **Config Struct over Options** - Replace verbose generic options with non-generic config
2. **Simplified Cancel Path** - Remove dedicated cancel goroutine
3. **Clear Middleware Separation** - Distinguish config from behavioral middleware
4. **Enhanced Fan-Out** - Configurable broadcast with slow-receiver handling
5. **Unified Source Interface** - Generator and Subscriber share common Source interface

## Implementation Order

This feature should be implemented in phases:

### Phase 0: Core Pipe Refactoring (This Feature)

**Must complete before CloudEvents standardization:**

```
Phase 0: Core Refactoring
├── 0.1: ProcessorConfig struct
├── 0.2: Simplified cancel path
├── 0.3: Middleware separation
├── 0.4: FanOut enhancement
├── 0.5: Source interface (Generator/Subscriber)
└── 0.6: Migration utilities
```

### Phase 1-7: CloudEvents Standardization

After core refactoring, proceed with ADRs 0019-0025.

## Key Changes

### 1. ProcessorConfig Struct

**Before (verbose generic options):**
```go
pipe := NewProcessPipe(
    handler,
    WithConcurrency[Order, ShippingCommand](4),
    WithBuffer[Order, ShippingCommand](100),
    WithTimeout[Order, ShippingCommand](5*time.Second),
    WithRetryConfig[Order, ShippingCommand](RetryConfig{...}),
)
```

**After (non-generic config):**
```go
pipe := NewProcessPipe(handler, ProcessorConfig{
    Concurrency: 4,
    Buffer:      100,
    Timeout:     5 * time.Second,
    OnError: func(input any, err error) {
        log.Printf("error: %v", err)
    },
})
```

### 2. Config vs Middleware Table

| Concern | Config Struct | Middleware |
|---------|---------------|------------|
| Worker count | `Concurrency` | - |
| Channel buffer | `Buffer` | - |
| Error callback | `OnError` | - |
| Panic callback | `OnPanic` | - |
| Panic recovery | `RecoverPanics` | - |
| **Timeout** | - | `WithTimeout(d)` |
| **Retry** | - | `WithRetry(cfg)` |
| **Metrics** | - | `WithMetrics(c)` |
| **Logging** | - | `WithLogging(l)` |
| **Tracing** | - | `WithTracing(t)` |

**Rule:** Config = static settings, Middleware = wraps Process()

### 3. Simplified Processor Interface

```go
// No Cancel method - use OnError callback instead
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}
```

### 4. New Function Signatures

```go
// ProcessPipe with config and middleware
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    config ProcessorConfig,
    middleware ...Middleware[In, Out],
) Pipe[In, Out]

// Generator with config
func NewGenerator[Out any](
    generate func(context.Context) ([]Out, error),
    config GeneratorConfig,
) Source[Out]

// Ticker generator
func NewTickerGenerator[Out any](
    interval time.Duration,
    generate func(ctx context.Context, tick time.Time) ([]Out, error),
    config GeneratorConfig,
) Source[Out]

// Enhanced broadcast
func BroadcastWithConfig[T any](
    in <-chan T,
    n int,
    config BroadcastConfig,
) []<-chan T
```

### 5. Source Interface

```go
// Source produces values without input (Generator, Subscriber)
type Source[Out any] interface {
    Start(ctx context.Context) <-chan Out
}
```

## Implementation Steps

### Step 0.1: ProcessorConfig

1. Create `ProcessorConfig` struct with all settings
2. Create `DefaultProcessorConfig` with sensible defaults
3. Add validation for config values
4. Update `StartProcessor` to accept config

**Files:**
- `processor_config.go` (new)
- `processor.go` (modify)

### Step 0.2: Simplified Cancel Path

1. Remove dedicated cancel goroutine from `StartProcessor`
2. Simplify to single worker loop with context checking
3. Add `OnError` callback invocation
4. Remove `Cancel` method from Processor interface

**Files:**
- `processor.go` (modify)
- `processor_test.go` (modify)

### Step 0.3: Middleware Package Consolidation

Consolidate all middleware into the dedicated `middleware/` package with clear organization.

#### Target Package Structure

```
middleware/
├── middleware.go       # Core types, interfaces, Chain()
├── timeout.go          # Context timeout middleware
├── retry.go            # Retry middleware (moved from root)
├── metrics.go          # Metrics collection middleware
├── logging.go          # Logging middleware (slog, zap adapters)
├── recover.go          # Panic recovery middleware
├── tracing.go          # OpenTelemetry tracing middleware
├── message/            # Message-specific middleware
│   ├── correlation.go  # Correlation ID propagation (exists)
│   ├── validation.go   # CloudEvents validation
│   └── routing.go      # Error routing middleware
└── doc.go              # Package documentation
```

#### Sub-steps

**0.3.1: Core Middleware Types**

Create `middleware/middleware.go`:
```go
package middleware

// Chain combines multiple middleware into one
func Chain[In, Out any](mw ...Middleware[In, Out]) Middleware[In, Out]

// Type aliases for convenience
type Middleware[In, Out any] = gopipe.Middleware[In, Out]
type MiddlewareFunc[In, Out any] = func(gopipe.Processor[In, Out]) gopipe.Processor[In, Out]
```

**0.3.2: Timeout Middleware**

Create `middleware/timeout.go`:
```go
type TimeoutConfig struct {
    Duration         time.Duration
    PropagateContext bool // default: true
}

func WithTimeout[In, Out any](config TimeoutConfig) Middleware[In, Out]
```

**0.3.3: Move Retry Middleware**

Move from `gopipe/retry.go` to `middleware/retry.go`:
```go
type RetryConfig struct {
    ShouldRetry ShouldRetryFunc
    Backoff     BackoffFunc
    MaxAttempts int
    Timeout     time.Duration
}

func WithRetry[In, Out any](config RetryConfig) Middleware[In, Out]

// Backoff strategies (moved)
func ConstantBackoff(delay time.Duration, jitter float64) BackoffFunc
func ExponentialBackoff(initial, factor, max time.Duration, jitter float64) BackoffFunc
```

**0.3.4: Metrics Middleware**

Create `middleware/metrics.go`:
```go
type MetricsCollector interface {
    RecordProcessed(duration time.Duration, success bool, labels map[string]string)
    RecordInFlight(delta int, labels map[string]string)
}

type MetricsConfig struct {
    Collector   MetricsCollector
    LabelFunc   func(in any) map[string]string
}

func WithMetrics[In, Out any](config MetricsConfig) Middleware[In, Out]

// Prometheus integration
func NewPrometheusCollector(namespace, subsystem string) *PrometheusCollector
```

**0.3.5: Logging Middleware**

Create `middleware/logging.go`:
```go
type Logger interface {
    Log(level LogLevel, msg string, fields map[string]any)
}

type LoggingConfig struct {
    Logger      Logger
    Level       LogLevel
    LogInput    bool
    LogOutput   bool
    LogDuration bool
}

func WithLogging[In, Out any](config LoggingConfig) Middleware[In, Out]

// Adapters
func SlogAdapter(logger *slog.Logger) Logger
func ZapAdapter(logger *zap.Logger) Logger
```

**0.3.6: Recovery Middleware**

Create `middleware/recover.go`:
```go
type RecoverConfig struct {
    OnPanic     func(input any, recovered any, stack []byte)
    ReturnError bool
}

func WithRecover[In, Out any](config RecoverConfig) Middleware[In, Out]
```

**0.3.7: Message-Specific Middleware**

Enhance `middleware/message/`:
```go
// middleware/message/validation.go
func WithValidation(strict bool) Middleware[*Message, *Message]

// middleware/message/routing.go
type ErrorRoutingConfig struct {
    Destination  string // "gopipe://errors", "kafka://dlq"
    IncludeInput bool
    IncludeStack bool
}
func WithErrorRouting(config ErrorRoutingConfig) Middleware[*Message, *Message]
```

**0.3.8: Deprecate Root Package Functions**

Create `gopipe/deprecated.go`:
```go
// Deprecated: Use middleware.WithRetry instead
func WithRetryConfig[In, Out any](config RetryConfig) Option[In, Out]

// Deprecated: Use middleware.WithRecover instead
func WithRecover[In, Out any]() Option[In, Out]
```

**Files:**
- `middleware/middleware.go` (new)
- `middleware/timeout.go` (new)
- `middleware/retry.go` (new - moved from root)
- `middleware/metrics.go` (new)
- `middleware/logging.go` (new)
- `middleware/recover.go` (new)
- `middleware/message/validation.go` (new)
- `middleware/message/routing.go` (new)
- `middleware/message/correlation.go` (existing - keep)
- `middleware/message.go` (existing - keep)
- `deprecated.go` (new)
- `retry.go` (deprecate, keep for backward compat)
- `pipe.go` (modify signature)

### Step 0.4: FanOut Enhancement

1. Create `BroadcastConfig` struct
2. Add `BroadcastWithConfig` function
3. Add slow-receiver callback
4. Keep simple `Broadcast` for backward compatibility

**Files:**
- `channel/broadcast.go` (modify)
- `channel/broadcast_test.go` (modify)

### Step 0.5: Source Interface

1. Create `Source[Out]` interface
2. Create `GeneratorConfig` struct
3. Update Generator to use config
4. Add `NewTickerGenerator` and `NewPollingGenerator`
5. Update `message.Generator` to use new interface

**Files:**
- `source.go` (new)
- `generator.go` (modify)
- `generator_config.go` (new)
- `message/generator.go` (modify)

### Step 0.6: Migration Utilities

1. Create deprecated aliases for old functions
2. Add migration guide
3. Update examples

**Files:**
- `deprecated.go` (new)
- `docs/migration/core-refactoring.md` (new)

## Usage Examples

### Basic ProcessPipe

```go
pipe := NewProcessPipe(
    func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        return []ShippingCommand{{OrderID: order.ID}}, nil
    },
    ProcessorConfig{
        Concurrency: 4,
        Timeout:     5 * time.Second,
        OnError: func(in any, err error) {
            log.Printf("failed to process order: %v", err)
        },
    },
    WithRetry[Order, ShippingCommand](RetryConfig{MaxAttempts: 3}),
    WithMetrics[Order, ShippingCommand](metricsCollector),
)
```

### Time-Triggered Generator

```go
heartbeat := NewTickerGenerator(
    30*time.Second,
    func(ctx context.Context, tick time.Time) ([]*Message, error) {
        return []*Message{
            message.NewMessage(
                message.WithType("system.heartbeat"),
                message.WithSource("gopipe://service"),
                message.WithData(map[string]any{"timestamp": tick}),
            ),
        }, nil
    },
    GeneratorConfig{
        OnError:      func(err error) { log.Printf("heartbeat error: %v", err) },
        RetryOnError: true,
    },
)
```

### Enhanced Broadcast

```go
outputs := BroadcastWithConfig(events, 3, BroadcastConfig{
    Buffer:        10,
    SlowThreshold: time.Second,
    OnSlowReceiver: func(idx int, val any, elapsed time.Duration) {
        log.Printf("consumer %d blocked for %v", idx, elapsed)
    },
})
```

### Middleware Package Usage

```go
import (
    "github.com/fxsml/gopipe"
    "github.com/fxsml/gopipe/middleware"
    msgmw "github.com/fxsml/gopipe/middleware/message"
)

// Chain multiple middleware
pipe := gopipe.NewProcessPipe(
    handler,
    gopipe.ProcessorConfig{Concurrency: 4},
    middleware.Chain(
        middleware.WithTimeout[Order, ShippingCommand](middleware.TimeoutConfig{
            Duration: 5 * time.Second,
        }),
        middleware.WithRetry[Order, ShippingCommand](middleware.RetryConfig{
            MaxAttempts: 3,
            Backoff:     middleware.ExponentialBackoff(100*time.Millisecond, 2.0, 5*time.Second, 0.2),
        }),
        middleware.WithMetrics[Order, ShippingCommand](middleware.MetricsConfig{
            Collector: middleware.NewPrometheusCollector("gopipe", "orders"),
        }),
    ),
)

// Message-specific middleware
router.AddHandler("orders", orderHandler,
    gopipe.ProcessorConfig{Concurrency: 4},
    msgmw.WithCorrelation(),
    msgmw.WithValidation(true),
    msgmw.WithErrorRouting(msgmw.ErrorRoutingConfig{
        Destination:  "gopipe://dead-letters",
        IncludeInput: true,
    }),
)
```

## Testing Strategy

### Unit Tests

- `ProcessorConfig` validation
- `StartProcessor` with config
- Middleware ordering and composition
- `BroadcastWithConfig` slow receiver handling
- `NewTickerGenerator` timing accuracy
- `Source` interface implementations

### Integration Tests

- Full pipeline with config + middleware
- Generator → Processor → Output flow
- Broadcast to multiple consumers
- Error handling across pipeline

### Migration Tests

- Deprecated functions still work
- Old examples compile
- Behavior parity with old implementation

## Files Changed

### New Files

**Core:**
- `processor_config.go` - ProcessorConfig struct and defaults
- `source.go` - Source interface
- `generator_config.go` - GeneratorConfig struct
- `deprecated.go` - Backward compatibility aliases
- `docs/migration/core-refactoring.md` - Migration guide

**Middleware Package:**
- `middleware/middleware.go` - Core types, Chain(), type aliases
- `middleware/timeout.go` - Context timeout middleware
- `middleware/retry.go` - Retry middleware (moved from root)
- `middleware/metrics.go` - Metrics collection, PrometheusCollector
- `middleware/logging.go` - Logging middleware, slog/zap adapters
- `middleware/recover.go` - Panic recovery middleware
- `middleware/tracing.go` - OpenTelemetry tracing middleware (optional)
- `middleware/doc.go` - Package documentation

**Message Middleware:**
- `middleware/message/validation.go` - CloudEvents validation
- `middleware/message/routing.go` - Error routing middleware

### Modified Files

- `processor.go` - Simplified cancel path
- `pipe.go` - New signature with config + middleware
- `generator.go` - Config-based generator
- `channel/broadcast.go` - BroadcastWithConfig
- `message/generator.go` - Updated Generator interface
- `options.go` - Mark as deprecated
- `retry.go` - Deprecate, re-export from middleware
- `middleware/message.go` - Keep existing NewMessageMiddleware
- `middleware/correlation.go` - Keep existing, maybe move to message/

## Dependencies

This feature has no external dependencies but is a **prerequisite** for:

- Feature 09: CloudEvents Mandatory
- Feature 10: Non-Generic Message
- Feature 11: ContentType Serialization
- Feature 12: Internal Message Routing
- Feature 13: Internal Message Loop

## Related Features

- [Feature 08: Middleware Package](08-middleware-package.md) - Existing middleware
- [Feature 09: CloudEvents Mandatory](09-cloudevents-mandatory.md) - Uses simplified pipe

## Acceptance Criteria

### Core Refactoring
1. [ ] `ProcessorConfig` replaces generic options for settings
2. [ ] Cancel path simplified (no dedicated goroutine)
3. [ ] Clear separation between config and middleware
4. [ ] `BroadcastWithConfig` handles slow receivers
5. [ ] `Source` interface unifies Generator and Subscriber
6. [ ] `NewTickerGenerator` supports time-based generation

### Middleware Package
7. [ ] `middleware.Chain()` combines multiple middleware
8. [ ] `middleware.WithTimeout()` handles context timeouts
9. [ ] `middleware.WithRetry()` provides retry logic (moved from root)
10. [ ] `middleware.WithMetrics()` collects processing metrics
11. [ ] `middleware.WithLogging()` provides pluggable logging
12. [ ] `middleware.WithRecover()` handles panics
13. [ ] `middleware/message.WithCorrelation()` propagates correlation IDs
14. [ ] `middleware/message.WithValidation()` validates CloudEvents
15. [ ] `middleware/message.WithErrorRouting()` routes errors to destinations

### Migration
16. [ ] All existing tests pass with new implementation
17. [ ] Migration guide documents breaking changes
18. [ ] Deprecated functions work with deprecation warnings
19. [ ] Examples updated to use new API
