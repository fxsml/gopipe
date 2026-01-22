# Dynamic Worker Pool for gopipe

**Status:** In Progress

## Overview

Provide a unified worker pool abstraction for the `pipe` package that supports:
1. Static worker configuration (fixed concurrency)
2. Dynamic autoscaling based on load
3. Future: Preserved message ordering (see [pipe-ordering](pipe-ordering.md))

The API aligns with the `message` package's `PoolConfig` pattern for consistency.

## Design Approach

### Naming Convention (aligned with message package)

| message package | pipe package |
|-----------------|--------------|
| `PoolConfig.Workers` | `PoolConfig.Workers` |
| `PoolConfig.BufferSize` | `PoolConfig.BufferSize` |
| Named pools via `AddPoolWithConfig` | `Pool` struct for internal management |

### Backpressure-based scaling (validated against industry patterns)

| Library | Scale-Up Method | Scale-Down Method |
|---------|-----------------|-------------------|
| **Pond** | All workers busy + queue depth | Immediate on idle or IdleTimeout |
| **Ants** | Fixed capacity (no autoscale) | Periodic scavenger (ExpiryDuration, default 1s) |
| **workerpool-go** | Load avg > threshold (EWMA) | Load avg < threshold |
| **Watermill** | N/A (partition-based, implicit) | N/A |

**Our approach** (aligns with Pond's simpler model):
- Scale up: when all workers are busy (activeWorkers == totalWorkers) AND workers < max
- Scale down: when a worker has been idle for `ScaleDownAfter` AND workers > min
- Cooldown periods prevent thrashing

**Why not Watermill's approach?** Watermill relies on message broker partitions for parallelism. gopipe is a general-purpose pipeline library, so explicit worker management is more appropriate.

## Implementation

### File Structure

```
pipe/
├── pool.go                   # PoolConfig type with parse() method
├── autoscale.go              # AutoscaleConfig type with parse() method
├── processing.go             # Dispatcher + startStaticProcessing() + startAutoscaledProcessing()
└── internal/autoscale/
    ├── config.go             # Internal Config + Parse function
    ├── pool.go               # Pool struct, worker management, scaler loop
    └── pool_test.go          # Unit tests
```

### Configuration API

```go
// In pipe/pool.go
type PoolConfig struct {
    // Workers sets the number of concurrent workers.
    // Default is 1. Ignored when Autoscale is set.
    Workers int

    // Autoscale enables dynamic worker scaling based on load.
    // When set, Workers is ignored and workers scale between
    // MinWorkers and MaxWorkers based on backpressure.
    Autoscale *AutoscaleConfig

    // BufferSize sets the output channel buffer size.
    // Default is 0 (unbuffered).
    BufferSize int

    // ErrorHandler is called when processing fails.
    // Default logs via slog.Error.
    ErrorHandler func(in any, err error)

    // CleanupHandler is called when processing is complete.
    CleanupHandler func(ctx context.Context)

    // CleanupTimeout sets the timeout for cleanup operations.
    CleanupTimeout time.Duration

    // ShutdownTimeout controls shutdown behavior on context cancellation.
    // If <= 0, waits indefinitely for input to close naturally.
    // If > 0, waits up to this duration then forces shutdown.
    ShutdownTimeout time.Duration
}

// In pipe/autoscale.go
type AutoscaleConfig struct {
    MinWorkers        int           // Default: 1
    MaxWorkers        int           // Default: runtime.NumCPU()
    ScaleDownAfter    time.Duration // Default: 30s
    ScaleUpCooldown   time.Duration // Default: 5s
    ScaleDownCooldown time.Duration // Default: 10s
    CheckInterval     time.Duration // Default: 1s
}

var defaultAutoscaleConfig = AutoscaleConfig{
    MinWorkers:        1,
    ScaleDownAfter:    30 * time.Second,
    ScaleUpCooldown:   5 * time.Second,
    ScaleDownCooldown: 10 * time.Second,
    CheckInterval:     1 * time.Second,
}

func (c AutoscaleConfig) parse() AutoscaleConfig { ... }
```

### Usage Examples

```go
// Static workers (simple case)
p := pipe.NewProcessPipe(fn, pipe.PoolConfig{Workers: 4})

// Autoscaling workers
p := pipe.NewProcessPipe(fn, pipe.PoolConfig{
    Autoscale: &pipe.AutoscaleConfig{
        MinWorkers: 2,
        MaxWorkers: 16,
    },
})

// Future: Ordered processing (phase 2)
p := pipe.NewProcessPipe(fn, pipe.PoolConfig{
    Workers:       4,
    PreserveOrder: true,  // Outputs match input order
})
```

### Processing Dispatcher

```go
func startProcessing[In, Out any](...) <-chan Out {
    cfg = cfg.parse()

    if cfg.Autoscale != nil {
        return startAutoscaledProcessing(ctx, in, fn, cfg)
    }
    return startStaticProcessing(ctx, in, fn, cfg)
}
```

### Internal Pool (pipe/internal/autoscale/pool.go)

```go
type Config struct {
    MinWorkers, MaxWorkers        int
    ScaleDownAfter                time.Duration
    ScaleUpCooldown, ScaleDownCooldown time.Duration
    CheckInterval                 time.Duration
}

type Pool[In, Out any] struct {
    cfg           Config
    fn            func(context.Context, In) ([]Out, error)
    workers       map[int]*worker
    totalWorkers  atomic.Int64
    activeWorkers atomic.Int64
    // ...
}

func NewPool[In, Out any](cfg Config, fn func(...), ...) *Pool[In, Out]
func (p *Pool) Start(ctx context.Context, in <-chan In, bufferSize int) <-chan Out
func (p *Pool) Stop()
func (p *Pool) TotalWorkers() int64
func (p *Pool) ActiveWorkers() int64
```

## Default Values (0 = auto)

| Field | 0 means | Default value |
|-------|---------|---------------|
| Workers | use default | 1 |
| MinWorkers | use default | 1 |
| MaxWorkers | use default | runtime.NumCPU() |
| ScaleUpCooldown | use default | 5s |
| ScaleDownCooldown | use default | 10s |
| ScaleDownAfter | use default | 30s |
| CheckInterval | use default | 1s |

## Convention Alignment

The implementation follows repository conventions:

| Pattern | Implementation |
|---------|----------------|
| Config naming | `PoolConfig` aligns with message package |
| Field naming | `Workers` aligns with message package |
| Config defaults | `parse()` method applies defaults |
| Internal packages | `pipe/internal/autoscale/` for pool implementation |
| Dispatcher pattern | `startProcessing()` dispatches to `startStaticProcessing()` or `startAutoscaledProcessing()` |

## Verification

- **Unit tests**: 15+ tests covering min/max enforcement, scale-up triggers, scale-down on idle, cooldowns, goroutine leak detection
- **Benchmarks**: Comparison between static and autoscale processing under various loads
- **All tests pass**: `go test ./...` succeeds

## Future Enhancements

### Phase 2: Preserved Message Ordering

See [pipe-ordering.md](pipe-ordering.md) for the detailed plan.

The `PoolConfig` will be extended with:
```go
type PoolConfig struct {
    // ... existing fields ...

    // PreserveOrder enables in-order message delivery.
    // When true, outputs are reordered to match input sequence
    // despite parallel processing. Has memory/latency overhead.
    // Default: false
    PreserveOrder bool

    // OrderBufferSize is the max items to buffer while waiting
    // for in-sequence items. Only used when PreserveOrder is true.
    // Default: max(Workers, MaxWorkers) * 2
    OrderBufferSize int
}
```

### Other Future Enhancements (out of scope)

Based on research, these could be added later:
- **Strategies** (like Pond): Eager/Balanced/Lazy scaling aggressiveness
- **EWMA load averaging** (like workerpool-go): Smoother scaling decisions
- **Metrics callbacks**: OnScaleUp/OnScaleDown hooks for observability
- **Custom ScaleStrategy interface**: User-defined scaling logic (like gopool)
