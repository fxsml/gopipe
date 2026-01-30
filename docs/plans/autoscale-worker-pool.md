# Autoscale Worker Pool

**Status:** Proposed
**Related:** [HTTP CloudEvents Adapter](http-cloudevents-adapter.md)

## Overview

Dynamic worker scaling for pipes based on buffer pressure.

## Proposed Config

```go
type Config struct {
    MinWorkers      int           // floor (default: 1)
    MaxWorkers      int           // ceiling, 0 = unlimited
    ScaleUpThresh   float64       // buffer fill ratio to spawn (0.8)
    ScaleDownThresh float64       // buffer fill ratio to stop (0.2)
    ScaleInterval   time.Duration // check frequency (100ms)
}
```

## Behavior

```
buffer 80%+ full  → spawn worker (up to MaxWorkers)
buffer 20%- full  → stop worker (down to MinWorkers)
MaxWorkers=0      → unlimited spawning
```

## Use Cases

| Scenario | Config |
|----------|--------|
| HTTP ingest (bursty) | `MaxWorkers: 0` (unlimited) |
| External API (rate-limited) | `MaxWorkers: 5` |
| Background jobs | `MinWorkers: 1, MaxWorkers: 10` |

## Files

- `pipe/processing.go` — scaling in worker loop
- `pipe/autoscale.go` — algorithm (new)

## Acceptance Criteria

- [ ] Workers scale up when buffer fills
- [ ] Workers scale down when buffer drains
- [ ] Respects Min/Max bounds
- [ ] Graceful shutdown waits for in-flight
