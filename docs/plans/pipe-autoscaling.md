# Dynamic Concurrency Autoscaling for gopipe

**Status:** Implemented

## Overview

Replace the static `Concurrency int` configuration with a dynamic autoscaling system that supports min/max bounds and automatically adjusts worker count based on load.

## Current State

- `pipe/processing.go`: `Config.Concurrency` is a static int (default 1)
- Workers spawned once at startup: `wg.Add(cfg.Concurrency)` + `for range cfg.Concurrency`
- No runtime adjustment of worker count

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

## New Files

```
pipe/internal/autoscale/
├── pool.go       # Pool struct, worker management, Start/Stop
├── scaler.go     # Scaling decision loop
├── config.go     # Internal config with defaults
└── pool_test.go  # Unit tests
```

## Configuration API

```go
// In pipe/processing.go - add to Config struct
type Config struct {
    Concurrency int  // Static workers (ignored when Autoscale is set)

    // NEW: Dynamic scaling config
    Autoscale *AutoscaleConfig

    // ... existing fields unchanged ...
}

// New type in pipe/autoscale.go (or inline in processing.go)
type AutoscaleConfig struct {
    MinWorkers        int           // Default: 1
    MaxWorkers        int           // Default: runtime.NumCPU()
    ScaleDownAfter    time.Duration // Idle duration before scale-down. Default: 30s
    ScaleUpCooldown   time.Duration // Min time between scale-ups. Default: 5s
    ScaleDownCooldown time.Duration // Min time between scale-downs. Default: 10s
    CheckInterval     time.Duration // How often to evaluate. Default: 1s
}
```

## Usage Examples

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

## Implementation Steps

### 1. Create internal autoscale package
- `pipe/internal/autoscale/config.go`: Internal config struct with `parse()` method
- `pipe/internal/autoscale/pool.go`: Pool struct managing workers map, spawn/stop logic

### 2. Implement worker management
- Worker struct with id, stopCh, lastActive, idle state
- `spawnWorker()` / `stopWorker()` methods
- Idle tracking: mark active when processing, idle when waiting

### 3. Implement scaler loop
- `runScaler()` goroutine with ticker at CheckInterval
- Backpressure detection: scale up when `activeWorkers == totalWorkers` (all busy)
- Idle detection: scale down when worker's `lastActive` exceeds `ScaleDownAfter`
- Cooldown enforcement to prevent thrashing

### 4. Integrate with startProcessing()
- Check if `cfg.Autoscale != nil`
- If yes, delegate to `startAutoscaledProcessing()`
- If no, use existing static worker loop (unchanged)

### 5. Add tests
- Unit tests for pool scaling behavior
- Integration tests with varying load patterns
- Verify backward compatibility

## Key Files to Modify

| File | Change |
|------|--------|
| `pipe/processing.go` | Add `Autoscale *AutoscaleConfig` to Config, add `startAutoscaledProcessing()` |
| `pipe/internal/autoscale/pool.go` | NEW: Pool implementation |
| `pipe/internal/autoscale/scaler.go` | NEW: Scaling logic |
| `pipe/internal/autoscale/config.go` | NEW: Config with defaults |

## Load Detection (Pending Counter Approach)

The pending counter approach detects backpressure by tracking active vs total workers:

```go
type Pool struct {
    mu            sync.Mutex
    workers       map[int]*worker   // all spawned workers
    totalWorkers  atomic.Int64      // count of spawned workers
    activeWorkers atomic.Int64      // workers currently processing (not idle)
}

type worker struct {
    id         int
    lastActive time.Time     // last time worker finished processing
    stopCh     chan struct{} // signal to stop this worker
}
```

**Worker lifecycle:**
```go
func (p *Pool) runWorker(w *worker, in <-chan In, fn ProcessFunc) {
    for {
        select {
        case <-w.stopCh:
            return
        case val, ok := <-in:
            if !ok {
                return
            }
            p.activeWorkers.Add(1)       // mark as busy
            result, err := fn(ctx, val)
            p.activeWorkers.Add(-1)      // mark as available

            p.mu.Lock()
            w.lastActive = time.Now()    // track for idle detection
            p.mu.Unlock()

            // ... handle result/error ...
        }
    }
}
```

**Scale-up decision:**
```go
// If all workers are busy (activeWorkers == totalWorkers) for sustained period
if p.activeWorkers.Load() >= p.totalWorkers.Load() &&
   time.Since(lastScaleUp) >= cfg.ScaleUpCooldown &&
   p.totalWorkers.Load() < cfg.MaxWorkers {
    p.spawnWorker()
}
```

**Scale-down decision:**
```go
// Find workers idle for too long
for _, w := range p.workers {
    if time.Since(w.lastActive) >= cfg.ScaleDownAfter &&
       p.totalWorkers.Load() > cfg.MinWorkers {
        close(w.stopCh)  // gracefully stop this worker
        break  // scale down one at a time
    }
}
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

```go
func (c AutoscaleConfig) parse() AutoscaleConfig {
    if c.MinWorkers <= 0 {
        c.MinWorkers = 1
    }
    if c.MaxWorkers <= 0 {
        c.MaxWorkers = runtime.NumCPU()
    }
    // ... etc
}
```

## Verification

1. **Unit tests**: Test min/max enforcement, scale-up triggers, scale-down on idle, cooldowns
2. **Integration test**: Create pipe with autoscale, send burst of items, verify workers scale up, wait for idle, verify scale down
3. **Manual test**: Run example with logging to observe scaling behavior

## Future Enhancements (out of scope for v1)

Based on research, these could be added later:
- **Strategies** (like Pond): Eager/Balanced/Lazy scaling aggressiveness
- **EWMA load averaging** (like workerpool-go): Smoother scaling decisions
- **Metrics callbacks**: OnScaleUp/OnScaleDown hooks for observability
- **Custom ScaleStrategy interface**: User-defined scaling logic (like gopool)
