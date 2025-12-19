# ADR 0036: Logging and Metrics Middleware

**Date:** 2025-12-17
**Status:** Proposed

## Context

Observability (logging, metrics) is a cross-cutting concern that should be implemented as middleware rather than baked into processors.

## Decision

### Logging Middleware

```go
// middleware/logging.go
package middleware

// LogLevel defines logging verbosity
type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelWarn
    LogLevelError
)

// Logger interface for pluggable logging
type Logger interface {
    Log(level LogLevel, msg string, fields map[string]any)
}

// LoggingConfig configures logging behavior
type LoggingConfig struct {
    Logger      Logger
    Level       LogLevel
    LogInput    bool // Log input values (may be sensitive)
    LogOutput   bool // Log output values
    LogDuration bool // Log processing duration
}

// WithLogging creates logging middleware
func WithLogging[In, Out any](config LoggingConfig) Middleware[In, Out]

// Adapters for popular loggers
func SlogAdapter(logger *slog.Logger) Logger
func ZapAdapter(logger *zap.Logger) Logger
```

### Metrics Middleware

```go
// middleware/metrics.go
package middleware

// MetricsCollector receives processing metrics
type MetricsCollector interface {
    // RecordProcessed records a processing attempt
    RecordProcessed(duration time.Duration, success bool, labels map[string]string)
    // RecordInFlight tracks concurrent processing
    RecordInFlight(delta int, labels map[string]string)
}

// MetricsConfig configures metrics collection
type MetricsConfig struct {
    Collector   MetricsCollector
    LabelFunc   func(in any) map[string]string // Extract labels from input
    RecordInput bool                           // Include input in labels
}

// WithMetrics creates metrics middleware
func WithMetrics[In, Out any](config MetricsConfig) Middleware[In, Out]

// PrometheusCollector implements MetricsCollector for Prometheus
type PrometheusCollector struct {
    ProcessedTotal   *prometheus.CounterVec
    ProcessedLatency *prometheus.HistogramVec
    InFlight         *prometheus.GaugeVec
}

func NewPrometheusCollector(namespace, subsystem string) *PrometheusCollector
```

**Usage:**

```go
pipe := NewPipe(
    handler,
    ProcessorConfig{Concurrency: 4},
    WithLogging[Order, ShippingCommand](LoggingConfig{
        Logger:      SlogAdapter(slog.Default()),
        Level:       LogLevelInfo,
        LogDuration: true,
    }),
    WithMetrics[Order, ShippingCommand](MetricsConfig{
        Collector: NewPrometheusCollector("gopipe", "orders"),
    }),
)
```

## Consequences

**Positive:**
- Pluggable logging backends (slog, zap, zerolog)
- Pluggable metrics backends (Prometheus, StatsD, OpenTelemetry)
- Configurable verbosity and fields
- Composable with other middleware

**Negative:**
- Requires interface implementations for each backend
- Generic type parameters required per middleware instance
- Performance overhead for high-throughput systems

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0037](PRO-0037-middleware-package-consolidation.md) - Middleware Package Consolidation
