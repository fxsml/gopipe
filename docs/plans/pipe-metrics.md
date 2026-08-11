# Plan: Pipeline Metrics

**Status:** Complete

## Overview

Add observability to the `pipe` and `message` packages. Implemented in three layers:

- **Layer 1 — `pipe` package**: `Metrics` interface for push events (blocking time, processing); `Stats()` pull snapshots for buffer depth and inflight count.
- **Layer 2 — `message` package**: Propagate `Metrics`, `Labels`, and `LabelFunc` through `PipeConfig`, `MergerConfig`, and `DistributorConfig`. Default `LabelFunc` adds `cloudevents.type` dimension automatically.
- **Layer 3 — `message/otel`**: New Go module providing an OTel implementation of `pipe.Metrics` plus `ObserveStats` for pull-based buffer gauges.

## Design Goals

1. **Zero overhead when disabled**: `nil` Metrics = no timing, no allocations
2. **Backend agnostic**: `pipe.Metrics` interface works with OTel, Prometheus, logging, etc.
3. **Static + dynamic labels**: `Labels` map for static config-time dimensions; `LabelFunc` for per-message dynamic dimensions (e.g. CloudEvents type)
4. **Push/pull separation**: hot-path events pushed via `RecordProcessing`/`RecordWait`; buffer state pulled via `Stats()` and OTel observable gauges
5. **No Engine dependency**: Individual components (`Router`, `UnmarshalPipe`, `MarshalPipe`, `Merger`, `Distributor`) are instrumented directly

## Layer 1: `pipe` Package

### Interface (`pipe/metrics.go`)

```go
type Metrics interface {
    RecordProcessing(ctx context.Context, info ProcessingInfo)
    RecordWait(ctx context.Context, info WaitInfo)
}

type ProcessingInfo struct {
    Labels      map[string]string // merged static + dynamic labels
    Duration    time.Duration
    Error       error
    OutputCount int
}

type WaitInfo struct {
    Labels    map[string]string
    Operation string        // WaitOpReceive | WaitOpSend
    Duration  time.Duration
}

const (
    WaitOpReceive = "receive"
    WaitOpSend    = "send"
)
```

### Pull snapshots (`pipe/metrics.go`)

```go
type Stats struct {
    Depth    int               // current output buffer occupancy (len)
    Capacity int               // output buffer size (cap)
    Inflight int               // active workers (zero for Merger, Distributor)
    Labels   map[string]string // same source as push metric labels
}
```

`Stats()` is implemented on `ProcessPipe`, `BatchPipe`, `Merger`, and `Distributor`.

### Config fields added to `pipe.Config`, `pipe.MergerConfig`, `pipe.DistributorConfig`

```go
Metrics   Metrics
Labels    map[string]string
LabelFunc func(val any) map[string]string
```

`LabelFunc` is called once per received value, before `RecordProcessing` and the send-wait. Returned labels are merged with `Labels` (dynamic takes precedence on conflicts). Only called when `Metrics` is non-nil.

### Instrumentation points in `processing.go`

- `RecordWait(WaitOpReceive)` — after dequeuing from input
- `RecordProcessing` — after handler returns (duration includes send-wait for multi-output handlers)
- `RecordWait(WaitOpSend)` — after each successful send to output

## Layer 2: `message` Package

Propagated fields: `Metrics`, `Labels`, `LabelFunc` on `PipeConfig`, `MergerConfig`, `DistributorConfig`.

Default `LabelFunc` (set in each config's `parse()`):
```go
var messageLabelFunc = func(val any) map[string]string {
    msg, ok := val.(*Message)
    if !ok { return nil }
    t, _ := msg.Attributes[AttrType].(string)
    if t == "" { return nil }
    return map[string]string{"cloudevents.type": t}
}
```

`Stats()` method added to `Router`, `UnmarshalPipe`, `MarshalPipe`, `Merger`, `Distributor` — all delegate to the underlying `pipe` component.

## Layer 3: `message/otel` Module

New module: `github.com/fxsml/gopipe/message/otel`

### Push instruments (`PipeMetrics`)

| OTel name | Type | Unit | Description |
|---|---|---|---|
| `gopipe.receive.wait.duration` | Histogram | `s` | Time blocked on input channel |
| `gopipe.send.wait.duration` | Histogram | `s` | Time blocked sending to output |
| `gopipe.process.duration` | Histogram | `s` | Handler execution time |
| `gopipe.process.output` | Counter | `{message}` | Messages emitted by handler |
| `gopipe.process.total` | Counter | `{message}` | Messages processed; `error.type=""` on success, Go type string on failure |

All histograms share explicit buckets: `0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 100.0`

### Pull gauges (`ObserveStats`)

| OTel name | Type | Unit | Description |
|---|---|---|---|
| `gopipe.buffer.depth` | Observable Gauge | `{message}` | Output buffer occupancy |
| `gopipe.buffer.capacity` | Observable Gauge | `{message}` | Output buffer total capacity |
| `gopipe.inflight` | Observable Gauge | `{message}` | Active workers |

Registered via `PipeMetrics.ObserveStats(StatsProvider)`. Uses a single `RegisterCallback` to call `Stats()` once per collection cycle.

### Usage

```go
pm, err := msgotel.NewPipeMetrics(meter)
// ...

router := message.NewRouter(message.PipeConfig{
    Pool:    message.PoolConfig{Workers: 2, BufferSize: 64},
    Metrics: pm,
    Labels:  map[string]string{"pipeline": "orders", "stage": "router"},
})

// Register pull gauges — polled on each Prometheus scrape
if err := pm.ObserveStats(router); err != nil { ... }
```

## Implementation Status

- [x] `pipe/metrics.go` — `Metrics` interface, `Stats` struct, `mergeLabels`
- [x] `pipe/processing.go` — `LabelFunc` field, `mergeLabels` call, `RecordProcessing`, `RecordWait`
- [x] `pipe/merger.go` — `LabelFunc`, `RecordWait`, `Stats()`
- [x] `pipe/distributor.go` — `LabelFunc`, `RecordWait`, `Stats()`, `OutputStats()`
- [x] `pipe/pipe.go` — `Stats()` on `ProcessPipe` and `BatchPipe`, `atomic.Int64` inflight counter
- [x] `message/{pipes,router,merger,distributor}.go` — propagate fields, default `LabelFunc`, `Stats()`
- [x] `message/otel/pipe_metrics.go` — `PipeMetrics`, `ObserveStats`, `StatsProvider`
- [x] `message/otel/stats.go` — `StatsProvider` interface
- [x] `examples/08-otel-metrics/` — Prometheus exporter example

## Design Decisions

See [pipe-metrics.decisions.md](pipe-metrics.decisions.md).

## Backward Compatibility

All new fields (`Metrics`, `Labels`, `LabelFunc`) have zero values that preserve existing behavior.
`Stats()` returns a zero `Stats` before the component is started.
