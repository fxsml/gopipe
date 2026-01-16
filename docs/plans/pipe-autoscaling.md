# Dynamic Concurrency Autoscaling for gopipe

**Status:** Implemented

## Overview

Replace the static `Concurrency int` configuration with a dynamic autoscaling system that supports min/max bounds and automatically adjusts worker count based on load.

## Design Approach

**Backpressure-based scaling** (validated against industry patterns):

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
├── autoscale.go              # AutoscaleConfig type with parse() method
├── processing.go             # Dispatcher + startStaticProcessing() + startAutoscaledProcessing()
└── internal/autoscale/
    ├── config.go             # Default constants only
    ├── pool.go               # Pool struct, worker management, scaler loop
    └── pool_test.go          # Unit tests
```

### Configuration API

```go
// In pipe/processing.go
type Config struct {
    Concurrency int              // Static workers (ignored when Autoscale is set)
    Autoscale   *AutoscaleConfig // Dynamic scaling config
    // ... existing fields unchanged ...
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
// Existing usage - unchanged (backward compatible)
p := pipe.NewProcessPipe(fn, pipe.Config{Concurrency: 4})

// New autoscaling usage
p := pipe.NewProcessPipe(fn, pipe.Config{
    Autoscale: &pipe.AutoscaleConfig{
        MinWorkers: 2,
        MaxWorkers: 16,
    },
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
type PoolConfig struct {
    MinWorkers, MaxWorkers        int
    ScaleDownAfter                time.Duration
    ScaleUpCooldown, ScaleDownCooldown time.Duration
    CheckInterval                 time.Duration
}

type Pool[In, Out any] struct {
    cfg           PoolConfig
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

## Default Values (0 = auto)

| Field | 0 means | Default value |
|-------|---------|---------------|
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
| Config defaults | `defaultAutoscaleConfig` variable + `parse()` method |
| Internal packages | `pipe/internal/autoscale/` for pool implementation |
| Dispatcher pattern | `startProcessing()` dispatches to `startStaticProcessing()` or `startAutoscaledProcessing()` |
| No type duplication | Single `AutoscaleConfig` in pipe package, `PoolConfig` in internal |

## Verification

- **Unit tests**: 15+ tests covering min/max enforcement, scale-up triggers, scale-down on idle, cooldowns, goroutine leak detection
- **Benchmarks**: Comparison between static and autoscale processing under various loads
- **All tests pass**: `go test ./...` succeeds

## Future Enhancements (out of scope for v1)

Based on research, these could be added later:
- **Strategies** (like Pond): Eager/Balanced/Lazy scaling aggressiveness
- **EWMA load averaging** (like workerpool-go): Smoother scaling decisions
- **Metrics callbacks**: OnScaleUp/OnScaleDown hooks for observability
- **Custom ScaleStrategy interface**: User-defined scaling logic (like gopool)
