# Pipeline Metrics for gopipe

**Status:** Planned

## Overview

Add observability to the `pipe` package with metrics for:
- **Backpressure detection**: Channel wait times (receive/send blocking)
- **Queue monitoring**: Output channel buffer utilization
- **Processing metrics**: Duration, errors, throughput

The design supports future extension for worker pool metrics (Phase 2).

## Design Goals

1. **Zero overhead when disabled**: `nil` Metrics = no timing, no allocations
2. **Forward compatible**: Phase 2 pool metrics via optional interface extension
3. **Backend agnostic**: Interface works with OTel, Prometheus, logging, etc.
4. **Correlation friendly**: Static labels for metrics/logging correlation

## Phase 1: Core Metrics

### Interface

```go
// pipe/metrics.go

// Metrics receives pipeline observability events.
// All methods must be safe for concurrent use.
type Metrics interface {
    // RecordProcessing is called after each item is processed.
    RecordProcessing(ctx context.Context, info ProcessingInfo)

    // RecordWait is called after blocking on a channel operation.
    RecordWait(ctx context.Context, info WaitInfo)

    // RecordQueue is called to report output channel buffer state.
    RecordQueue(ctx context.Context, info QueueInfo)
}

// ProcessingInfo contains metrics for a processed item.
type ProcessingInfo struct {
    Labels      map[string]string // Static labels from Config
    Duration    time.Duration     // Handler execution time
    Error       error             // nil on success
    InputCount  int               // 1 for single, N for batch
    OutputCount int               // Items produced
}

// WaitInfo contains metrics for a channel wait.
type WaitInfo struct {
    Labels    map[string]string
    Operation string        // "receive" | "send"
    Duration  time.Duration // Time blocked on channel
}

// QueueInfo contains output channel buffer state.
type QueueInfo struct {
    Labels   map[string]string
    Depth    int // Current items in buffer (len)
    Capacity int // Buffer size (cap)
}
```

### Config Extension

```go
// pipe/processing.go

type Config struct {
    Concurrency     int
    BufferSize      int
    ProcessTimeout  time.Duration
    ShutdownTimeout time.Duration
    ErrorHandler    func(in any, err error)
    CleanupHandler  func(ctx context.Context)
    CleanupTimeout  time.Duration

    // Metrics receives observability events (optional, nil = no overhead).
    Metrics Metrics

    // Labels provides static identifiers for metrics and logging.
    // Set once at configuration time, same for all operations.
    // Common: "stage", "pipeline", "handler", "service", "env"
    Labels map[string]string
}
```

### Metrics Collected

| Category | Metric | Type | Description |
|----------|--------|------|-------------|
| **Backpressure** | `receive_wait` | Histogram | Time blocked waiting for input |
| **Backpressure** | `send_wait` | Histogram | Time blocked sending output |
| **Queue** | `queue_depth` | Gauge | Current items in output buffer |
| **Queue** | `queue_capacity` | Gauge | Output buffer size |
| **Processing** | `duration` | Histogram | Handler execution time |
| **Processing** | `errors` | Counter | Processing failures |
| **Processing** | `input_count` | Counter | Items received |
| **Processing** | `output_count` | Counter | Items produced |

### Implementation Points

#### processing.go

```go
func startProcessing[In, Out any](ctx context.Context, in <-chan In, fn ProcessFunc[In, Out], cfg Config) <-chan Out {
    cfg = cfg.parse()
    out := make(chan Out, cfg.BufferSize)

    // ... existing setup ...

    for range cfg.Concurrency {
        go func() {
            for {
                // === METRICS: Receive wait ===
                var receiveStart time.Time
                if cfg.Metrics != nil {
                    receiveStart = time.Now()
                }

                select {
                case val, ok := <-in:
                    if cfg.Metrics != nil {
                        cfg.Metrics.RecordWait(ctx, WaitInfo{
                            Labels:    cfg.Labels,
                            Operation: "receive",
                            Duration:  time.Since(receiveStart),
                        })
                    }

                    if !ok {
                        return
                    }

                    // === METRICS: Processing ===
                    var processStart time.Time
                    if cfg.Metrics != nil {
                        processStart = time.Now()
                    }

                    res, err := fn(handlerCtx, val)

                    if cfg.Metrics != nil {
                        cfg.Metrics.RecordProcessing(ctx, ProcessingInfo{
                            Labels:      cfg.Labels,
                            Duration:    time.Since(processStart),
                            Error:       err,
                            InputCount:  1,
                            OutputCount: len(res),
                        })
                    }

                    // === METRICS: Send wait ===
                    for _, r := range res {
                        var sendStart time.Time
                        if cfg.Metrics != nil {
                            sendStart = time.Now()
                        }

                        select {
                        case out <- r:
                            if cfg.Metrics != nil {
                                cfg.Metrics.RecordWait(ctx, WaitInfo{
                                    Labels:    cfg.Labels,
                                    Operation: "send",
                                    Duration:  time.Since(sendStart),
                                })
                                cfg.Metrics.RecordQueue(ctx, QueueInfo{
                                    Labels:   cfg.Labels,
                                    Depth:    len(out),
                                    Capacity: cap(out),
                                })
                            }
                        case <-done:
                            // ...
                        }
                    }

                case <-done:
                    return
                }
            }
        }()
    }

    // ...
}
```

#### merger.go

Add metrics to the merge worker loop:
- `RecordWait` for sends to output channel
- `RecordQueue` for output channel state

#### distributor.go

Add metrics to the route function:
- `RecordWait` for sends to output channels
- `RecordQueue` for each output channel state

### Files Changed

| File | Changes |
|------|---------|
| `pipe/metrics.go` | **NEW** - Interface and info types |
| `pipe/processing.go` | Add `Metrics`, `Labels` to Config; instrument worker loop |
| `pipe/merger.go` | Add metrics to `MergerConfig`; instrument merge loop |
| `pipe/distributor.go` | Add metrics to `DistributorConfig`; instrument route |
| `pipe/errors.go` | No changes |

### Usage Example

```go
// Simple logging implementation
type LogMetrics struct {
    logger *slog.Logger
}

func (m *LogMetrics) RecordProcessing(ctx context.Context, info ProcessingInfo) {
    m.logger.Debug("processed",
        slog.String("stage", info.Labels["stage"]),
        slog.Duration("duration", info.Duration),
        slog.Int("outputs", info.OutputCount),
    )
}

func (m *LogMetrics) RecordWait(ctx context.Context, info WaitInfo) {
    if info.Duration > 100*time.Millisecond {
        m.logger.Warn("channel wait",
            slog.String("stage", info.Labels["stage"]),
            slog.String("op", info.Operation),
            slog.Duration("duration", info.Duration),
        )
    }
}

func (m *LogMetrics) RecordQueue(ctx context.Context, info QueueInfo) {
    saturation := float64(info.Depth) / float64(info.Capacity)
    if saturation > 0.8 {
        m.logger.Warn("queue saturation",
            slog.String("stage", info.Labels["stage"]),
            slog.Float64("saturation", saturation),
        )
    }
}

// Usage
p := pipe.NewProcessPipe(handler, pipe.Config{
    Concurrency: 4,
    BufferSize:  100,
    Metrics:     &LogMetrics{logger: slog.Default()},
    Labels: map[string]string{
        "stage":    "router",
        "pipeline": "order-processing",
        "handler":  "validate-order",
    },
})
```

## Phase 2: Pool Metrics (Future)

Extends Phase 1 with worker pool observability. Implemented alongside `feature/worker-pool`.

### Extended Interface

```go
// PoolMetrics extends Metrics with pool observability.
// Implementations may optionally implement this interface.
type PoolMetrics interface {
    Metrics

    // RecordPoolState is called periodically to report pool state.
    RecordPoolState(ctx context.Context, info PoolStateInfo)
}

// PoolStateInfo contains worker pool state.
type PoolStateInfo struct {
    Labels        map[string]string
    TotalWorkers  int     // Current worker count
    ActiveWorkers int     // Workers currently processing
    MinWorkers    int     // Configured minimum
    MaxWorkers    int     // Configured maximum
    Utilization   float64 // active/total (0.0-1.0)
    Saturation    float64 // active/max (0.0-1.0)
    ScaleEvent    string  // "" | "scale_up" | "scale_down"
}
```

### Usage in Autoscale Pool

```go
// In internal/autoscale/pool.go
func (p *Pool) reportState(ctx context.Context, event string) {
    if p.metrics == nil {
        return
    }

    // Check for optional PoolMetrics interface
    if pm, ok := p.metrics.(PoolMetrics); ok {
        pm.RecordPoolState(ctx, PoolStateInfo{
            Labels:        p.labels,
            TotalWorkers:  int(p.totalWorkers.Load()),
            ActiveWorkers: int(p.activeWorkers.Load()),
            MinWorkers:    p.cfg.MinWorkers,
            MaxWorkers:    p.cfg.MaxWorkers,
            Utilization:   float64(active) / float64(total),
            Saturation:    float64(active) / float64(p.cfg.MaxWorkers),
            ScaleEvent:    event,
        })
    }
}
```

## Backward Compatibility

| Change | Impact |
|--------|--------|
| New `Metrics` field in Config | None - zero value (nil) preserves existing behavior |
| New `Labels` field in Config | None - zero value (nil) works fine |
| Phase 2 `PoolMetrics` interface | None - optional interface, type assertion |

## Testing Strategy

1. **Unit tests**: Verify metrics are called with correct values
2. **Nil safety**: Verify no panics when `Metrics` is nil
3. **Concurrency**: Verify thread-safety of metrics calls
4. **Benchmarks**: Compare overhead with/without metrics enabled

## Open Questions

1. **Queue sampling frequency**: Currently on every send. Consider sampling interval?
2. **Batch metrics**: Should `InputCount` be batch size for `BatchPipe`? (Currently proposed: yes)
3. **Error categorization**: Should we distinguish error types in metrics?
