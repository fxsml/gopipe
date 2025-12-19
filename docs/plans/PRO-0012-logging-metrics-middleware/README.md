# PRO-0012: Logging and Metrics Middleware

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0036

## Overview

Implement pluggable logging and metrics middleware for observability.

## Goals

1. Create logging middleware with pluggable backends
2. Create metrics middleware with pluggable collectors
3. Provide adapters for popular libraries

## Task 1: Logging Middleware

**Goal:** Implement `WithLogging` middleware

```go
// middleware/logging.go
type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelWarn
    LogLevelError
)

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

**Files to Create/Modify:**
- `middleware/logging.go` (new) - Logging middleware
- `middleware/logging_adapters.go` (new) - Logger adapters

## Task 2: Metrics Middleware

**Goal:** Implement `WithMetrics` middleware

```go
// middleware/metrics.go
type MetricsCollector interface {
    RecordProcessed(duration time.Duration, success bool, labels map[string]string)
    RecordInFlight(delta int, labels map[string]string)
}

type MetricsConfig struct {
    Collector   MetricsCollector
    LabelFunc   func(in any) map[string]string
    RecordInput bool
}

func WithMetrics[In, Out any](config MetricsConfig) Middleware[In, Out]

type PrometheusCollector struct {
    ProcessedTotal   *prometheus.CounterVec
    ProcessedLatency *prometheus.HistogramVec
    InFlight         *prometheus.GaugeVec
}

func NewPrometheusCollector(namespace, subsystem string) *PrometheusCollector
```

**Files to Create/Modify:**
- `middleware/metrics.go` (new) - Metrics middleware
- `middleware/metrics_prometheus.go` (new) - Prometheus collector

**Acceptance Criteria:**
- [ ] `Logger` interface defined
- [ ] `WithLogging` middleware implemented
- [ ] slog and zap adapters implemented
- [ ] `MetricsCollector` interface defined
- [ ] `WithMetrics` middleware implemented
- [ ] `PrometheusCollector` implemented
- [ ] Tests for both middleware
- [ ] CHANGELOG updated

## Related

- [PRO-0036](../../adr/PRO-0036-logging-metrics-middleware.md) - ADR
- [PRO-0013](../PRO-0013-middleware-package-consolidation/) - Middleware package
