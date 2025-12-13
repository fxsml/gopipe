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

### Step 0.3: Middleware Separation

1. Extract timeout, retry from options to middleware
2. Create middleware package with standard middleware
3. Update `NewProcessPipe` signature
4. Document middleware vs config distinction

**Files:**
- `pipe.go` (modify)
- `middleware/timeout.go` (new or modify)
- `middleware/retry.go` (new or modify)
- `middleware/metrics.go` (new)
- `middleware/logging.go` (new)

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

- `processor_config.go` - ProcessorConfig struct and defaults
- `source.go` - Source interface
- `generator_config.go` - GeneratorConfig struct
- `deprecated.go` - Backward compatibility aliases
- `middleware/metrics.go` - Metrics middleware
- `middleware/logging.go` - Logging middleware
- `docs/migration/core-refactoring.md` - Migration guide

### Modified Files

- `processor.go` - Simplified cancel path
- `pipe.go` - New signature with config + middleware
- `generator.go` - Config-based generator
- `channel/broadcast.go` - BroadcastWithConfig
- `message/generator.go` - Updated Generator interface
- `options.go` - Mark as deprecated

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

1. [ ] `ProcessorConfig` replaces generic options for settings
2. [ ] Cancel path simplified (no dedicated goroutine)
3. [ ] Clear separation between config and middleware
4. [ ] `BroadcastWithConfig` handles slow receivers
5. [ ] `Source` interface unifies Generator and Subscriber
6. [ ] `NewTickerGenerator` supports time-based generation
7. [ ] All existing tests pass with new implementation
8. [ ] Migration guide documents breaking changes
9. [ ] Examples updated to use new API
