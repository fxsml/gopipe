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

### Unified Config: Workers + MaxWorkers

The key insight is that `Workers` serves dual purpose:
- **Static mode**: The fixed worker count
- **Autoscale mode**: The minimum worker count (floor)

Autoscaling is enabled when `MaxWorkers > Workers`. This eliminates the need for a separate `MinWorkers` field or nested `AutoscaleConfig` struct.

| Workers | MaxWorkers | Mode | Result |
|---------|------------|------|--------|
| 0 | 0 | static | 1 worker (default) |
| 4 | 0 | static | 4 workers |
| 4 | 4 | static | 4 workers |
| 2 | 16 | autoscale | 2-16 workers |

### Backpressure-based scaling (validated against industry patterns)

| Library | Scale-Up Method | Scale-Down Method |
|---------|-----------------|-------------------|
| **Pond** | All workers busy + queue depth | Immediate on idle or IdleTimeout |
| **Ants** | Fixed capacity (no autoscale) | Periodic scavenger (ExpiryDuration, default 1s) |
| **workerpool-go** | Load avg > threshold (EWMA) | Load avg < threshold |
| **Watermill** | N/A (partition-based, implicit) | N/A |

**Our approach** (aligns with Pond's simpler model):
- Scale up: when all workers are busy (activeWorkers == totalWorkers) AND workers < MaxWorkers
- Scale down: when a worker has been idle for `ScaleDownAfter` AND workers > Workers
- Cooldown periods prevent thrashing

**Why not Watermill's approach?** Watermill relies on message broker partitions for parallelism. gopipe is a general-purpose pipeline library, so explicit worker management is more appropriate.

## Implementation

### File Structure

```
pipe/
├── pool.go                   # PoolConfig type with parse() and isAutoscale()
├── processing.go             # Config + startProcessing() + startStaticProcessing() + startAutoscaledProcessing()
└── internal/autoscale/
    ├── config.go             # Default constants
    ├── pool.go               # Pool struct, worker management, scaler loop
    └── pool_test.go          # Unit tests
```

### Configuration API

```go
// In pipe/pool.go
type PoolConfig struct {
    // Workers sets worker count (static mode) or minimum workers (autoscale mode).
    // Default: 1
    Workers int

    // MaxWorkers enables autoscaling when > Workers.
    // Workers scale between Workers and MaxWorkers based on backpressure.
    // If <= Workers (including 0), uses static mode with Workers count.
    // Default: Workers (static mode)
    MaxWorkers int

    // BufferSize sets the output channel buffer size.
    // Default: 0 (unbuffered)
    BufferSize int

    // Autoscale timing (only used when MaxWorkers > Workers)
    ScaleDownAfter    time.Duration // Default: 30s
    ScaleUpCooldown   time.Duration // Default: 5s
    ScaleDownCooldown time.Duration // Default: 10s
    CheckInterval     time.Duration // Default: 1s
}

func (c PoolConfig) isAutoscale() bool {
    return c.MaxWorkers > c.Workers
}

// In pipe/processing.go
type Config struct {
    // Pool configures the worker pool (worker count, autoscaling, and buffer size).
    Pool PoolConfig

    // ProcessTimeout sets a per-message processing deadline (default: 0, no timeout).
    ProcessTimeout time.Duration

    // ErrorHandler is called when processing fails.
    // Default logs via slog.Error.
    ErrorHandler func(in any, err error)

    // CleanupHandler is called when processing is complete.
    CleanupHandler func(ctx context.Context)

    // CleanupTimeout sets the timeout for cleanup operations.
    CleanupTimeout time.Duration

    // ShutdownTimeout controls shutdown behavior on context cancellation.
    // If <= 0, forces immediate shutdown (no grace period).
    // If > 0, waits up to this duration for natural completion, then forces shutdown.
    ShutdownTimeout time.Duration
}
```

### Usage Examples

```go
// Static workers (simple case)
p := pipe.NewProcessPipe(fn, pipe.Config{
    Pool: pipe.PoolConfig{Workers: 4},
})

// Autoscaling workers (MaxWorkers > Workers enables it)
p := pipe.NewProcessPipe(fn, pipe.Config{
    Pool: pipe.PoolConfig{Workers: 2, MaxWorkers: 16},
})

// Default (1 static worker)
p := pipe.NewProcessPipe(fn, pipe.Config{})

// With buffer and timing overrides
p := pipe.NewProcessPipe(fn, pipe.Config{
    Pool: pipe.PoolConfig{
        Workers:           2,
        MaxWorkers:        16,
        BufferSize:        100,
        ScaleUpCooldown:   2 * time.Second,
        ScaleDownCooldown: 5 * time.Second,
        ScaleDownAfter:    15 * time.Second,
        CheckInterval:     500 * time.Millisecond,
    },
})

// Future: Ordered processing (phase 2)
p := pipe.NewProcessPipe(fn, pipe.Config{
    Pool: pipe.PoolConfig{Workers: 4, PreserveOrder: true},
})
```

### Processing Dispatcher

```go
func startProcessing[In, Out any](...) <-chan Out {
    cfg = cfg.parse()

    if cfg.Pool.isAutoscale() {
        return startAutoscaledProcessing(ctx, in, fn, cfg)
    }
    return startStaticProcessing(ctx, in, fn, cfg)
}
```

### Internal Pool (pipe/internal/autoscale/pool.go)

```go
type PoolConfig struct {
    MinWorkers, MaxWorkers             int
    ScaleDownAfter                     time.Duration
    ScaleUpCooldown, ScaleDownCooldown time.Duration
    CheckInterval                      time.Duration
}

type Pool[In, Out any] struct {
    cfg           PoolConfig
    ctx           context.Context // stored for spawning workers during scaling
    fn            func(context.Context, In) ([]Out, error)
    workers       map[int]*worker
    totalWorkers  atomic.Int64
    activeWorkers atomic.Int64
    // ...
}

func NewPool[In, Out any](cfg PoolConfig, fn func(...), ...) *Pool[In, Out]
func (p *Pool) Start(ctx context.Context, in <-chan In, bufferSize int) <-chan Out
func (p *Pool) Stop()
func (p *Pool) TotalWorkers() int64
func (p *Pool) ActiveWorkers() int64
```

Note: Internal pool uses `MinWorkers` which maps from `PoolConfig.Workers`.

## Default Values (0 = use default)

| Field | 0 means | Default value |
|-------|---------|---------------|
| Workers | use default | 1 |
| MaxWorkers | use Workers | Workers (static mode) |
| BufferSize | unbuffered | 0 |
| ScaleUpCooldown | use default | 5s |
| ScaleDownCooldown | use default | 10s |
| ScaleDownAfter | use default | 30s |
| CheckInterval | use default | 1s |

## Shutdown Behavior

Both static and autoscaled paths have identical shutdown semantics:

| ShutdownTimeout | Behavior |
|-----------------|----------|
| <= 0 | Immediate forced shutdown — workers cancel and exit |
| > 0 | Grace period: wait for natural completion OR timeout, then force |

On forced shutdown:
- Workers stop forwarding outputs
- Workers drain remaining input, calling `ErrorHandler` with `ErrShutdownDropped` for each
- Workers exit when input channel closes

`ProcessTimeout` is enforced independently in both paths. Per-message timeouts fire during grace periods; on forced shutdown all handlers are cancelled immediately.

## Convention Alignment

The implementation follows repository conventions:

| Pattern | Implementation |
|---------|----------------|
| Config naming | `PoolConfig` aligns with message package |
| Field naming | `Workers` aligns with message package |
| Separation of concerns | `PoolConfig` for workers, `Config` for pipe behavior |
| Config defaults | `parse()` method applies defaults |
| Internal packages | `pipe/internal/autoscale/` for pool implementation |
| Dispatcher pattern | `startProcessing()` dispatches based on `isAutoscale()` |

## Verification

- **Unit tests**: 15+ tests covering min/max enforcement, scale-up triggers, scale-down on idle, cooldowns, goroutine leak detection
- **Benchmarks**: Comparison between static and autoscale processing under various loads
- **Race detector**: `go test -race ./...` passes
- **Feature parity**: `ProcessTimeout`, `ShutdownTimeout`, `CleanupHandler` work identically in both static and autoscaled modes

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
    // Default: MaxWorkers * 2 (or Workers * 2 if static)
    OrderBufferSize int
}
```

### Other Future Enhancements (out of scope)

Based on research, these could be added later:
- **Strategies** (like Pond): Eager/Balanced/Lazy scaling aggressiveness
- **EWMA load averaging** (like workerpool-go): Smoother scaling decisions
- **Metrics callbacks**: OnScaleUp/OnScaleDown hooks for observability
- **Custom ScaleStrategy interface**: User-defined scaling logic (like gopool)
