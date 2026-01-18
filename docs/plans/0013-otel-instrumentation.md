# Plan: OpenTelemetry Instrumentation

**Status:** Proposed
**Depends On:** [Plan 0012](0012-graceful-loopback-shutdown.md) (MessageTracker)
**Related ADRs:** None (new feature)

## Overview

Add OpenTelemetry instrumentation to the message package for distributed tracing and metrics. Follows the pattern used by popular Go libraries (otelecho, otelgin, otelwatermill) with a separate instrumentation package.

## Goals

1. Distributed tracing through message handlers with proper span hierarchy
2. Metrics for message throughput, latency, and in-flight counts
3. Zero overhead when not configured (noop implementation)
4. Idiomatic otel integration following SDK conventions
5. Optional reuse of MessageTracker for in-flight metrics

## Research Findings

### Popular Go Library Patterns

| Library | Package | Pattern |
|---------|---------|---------|
| Echo | `otelecho` | Middleware, TracerProvider via options |
| Gin | `otelgin` | Middleware, TracerProvider via options |
| Watermill | `opentelemetry` | Publisher/Subscriber decorators |
| Go-kit | `tracing` | Endpoint middleware |
| gRPC | `otelgrpc` | Interceptors, Stats handlers |

### Key Patterns Identified

1. **Separate package**: `otel{name}` or `{name}/otel` keeps core dependency-free
2. **Provider injection**: Pass `TracerProvider`/`MeterProvider` via config
3. **Noop fallback**: Use `otel.GetTracerProvider()` if not specified
4. **Middleware pattern**: Wrap processing functions
5. **Semantic conventions**: Use otel semantic conventions for attributes

### Message Tracker Reuse

The `MessageTracker` from Plan 0012 tracks in-flight messages via `Enter()`/`Exit()`:

```go
type MessageTracker struct {
    inFlight atomic.Int64
    // ...
}

func (t *MessageTracker) InFlight() int64  // Already exposed for debugging
```

This can be reused for the `messaging.messages.inflight` metric, avoiding duplicate counting. However, the tracker is internal to Engine. Options:

1. **Expose InFlight()**: Add public accessor to Engine
2. **Callback hook**: MessageTracker calls a function on Enter/Exit
3. **Separate counter**: Telemetry maintains its own atomic counter

**Recommendation**: Option 2 (callback) is most flexible - allows multiple observers (metrics, logging, custom) without coupling.

## Package Structure

```
message/
├── otel/                      # OpenTelemetry instrumentation
│   ├── doc.go                 # Package documentation
│   ├── config.go              # OtelConfig, options
│   ├── middleware.go          # Tracing middleware for handlers
│   ├── metrics.go             # Metrics collector
│   ├── attributes.go          # Semantic attribute helpers
│   ├── propagation.go         # Context propagation helpers
│   └── middleware_test.go
└── middleware/
    └── (existing middleware)
```

## API Design

### Configuration

```go
package otel

import (
    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/trace"
)

// Config configures OpenTelemetry instrumentation.
type Config struct {
    // TracerProvider for creating spans. If nil, uses otel.GetTracerProvider().
    TracerProvider trace.TracerProvider
    // MeterProvider for creating metrics. If nil, uses otel.GetMeterProvider().
    MeterProvider metric.MeterProvider
    // ServiceName for span/metric attributes. Default: "gopipe".
    ServiceName string
    // Propagators for context propagation. If nil, uses otel.GetTextMapPropagator().
    Propagators propagation.TextMapPropagator
    // RecordInFlight enables in-flight message gauge. Default: true.
    RecordInFlight bool
}

// Option configures otel instrumentation.
type Option func(*Config)

// WithTracerProvider sets the tracer provider.
func WithTracerProvider(tp trace.TracerProvider) Option

// WithMeterProvider sets the meter provider.
func WithMeterProvider(mp metric.MeterProvider) Option

// WithServiceName sets the service name for attributes.
func WithServiceName(name string) Option

// WithPropagators sets context propagators.
func WithPropagators(p propagation.TextMapPropagator) Option
```

### Middleware

```go
// Middleware wraps message.ProcessFunc with tracing and metrics.
// Creates a span for each message processed and records metrics.
//
// Example:
//
//	engine.Use(otel.Middleware(
//	    otel.WithServiceName("order-service"),
//	))
func Middleware(opts ...Option) message.Middleware

// HandlerMiddleware wraps a Handler with tracing.
// Adds span around Handle() call with handler name and event type.
// Use when you need per-handler instrumentation.
//
// Example:
//
//	wrapped := otel.WrapHandler("process-orders", handler, opts...)
//	engine.AddHandler("process-orders", matcher, wrapped)
func WrapHandler(name string, h message.Handler, opts ...Option) message.Handler
```

### Metrics Collector

```go
// Collector records message processing metrics.
// Implements pipe/middleware.MetricsCollector for compatibility.
type Collector struct {
    // meters
}

// NewCollector creates a metrics collector.
func NewCollector(opts ...Option) (*Collector, error)

// Collect implements MetricsCollector interface.
func (c *Collector) Collect(m *middleware.Metrics)

// RecordInFlight updates the in-flight message gauge.
// Called by MessageTracker hooks when enabled.
func (c *Collector) RecordInFlight(delta int64)

// Close shuts down the collector.
func (c *Collector) Close() error
```

### Engine Plugin

```go
// Plugin registers OpenTelemetry instrumentation with the engine.
// Adds tracing middleware and metrics collection.
//
// Example:
//
//	engine.AddPlugin(otel.Plugin(
//	    otel.WithServiceName("order-service"),
//	))
func Plugin(opts ...Option) message.Plugin
```

## Semantic Conventions

Following [OpenTelemetry Messaging Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/messaging/):

### Span Attributes

| Attribute | Value | Description |
|-----------|-------|-------------|
| `messaging.system` | `"gopipe"` | Messaging system identifier |
| `messaging.operation` | `"process"` | Operation being performed |
| `messaging.message.id` | CloudEvents `id` | Message identifier |
| `messaging.message.type` | CloudEvents `type` | Event type |
| `messaging.source` | CloudEvents `source` | Event source |
| `service.name` | Configured value | Service name |

### Metrics

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `messaging.messages.processed` | Counter | `{message}` | Total messages processed |
| `messaging.messages.duration` | Histogram | `ms` | Processing duration |
| `messaging.messages.inflight` | UpDownCounter | `{message}` | Current in-flight count |
| `messaging.messages.errors` | Counter | `{message}` | Processing errors |

## Implementation

### Tracing Middleware

```go
func Middleware(opts ...Option) message.Middleware {
    cfg := newConfig(opts...)
    tracer := cfg.TracerProvider.Tracer("github.com/fxsml/gopipe/message/otel")

    return func(next message.ProcessFunc) message.ProcessFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            // Extract parent span from message attributes (if propagated)
            ctx = cfg.Propagators.Extract(ctx, messageCarrier{msg})

            // Start span
            ctx, span := tracer.Start(ctx, "gopipe.process",
                trace.WithSpanKind(trace.SpanKindConsumer),
                trace.WithAttributes(
                    semconv.MessagingSystem("gopipe"),
                    semconv.MessagingOperationProcess,
                    attribute.String("messaging.message.id", msg.ID()),
                    attribute.String("messaging.message.type", msg.Type()),
                    attribute.String("messaging.source", msg.Source()),
                ),
            )
            defer span.End()

            results, err := next(ctx, msg)

            if err != nil {
                span.RecordError(err)
                span.SetStatus(codes.Error, err.Error())
            }

            // Inject trace context into output messages for propagation
            for _, out := range results {
                cfg.Propagators.Inject(ctx, messageCarrier{out})
            }

            return results, err
        }
    }
}
```

### Message Carrier

```go
// messageCarrier adapts Message attributes for context propagation.
type messageCarrier struct {
    msg *message.Message
}

func (c messageCarrier) Get(key string) string {
    if v, ok := c.msg.Attributes[key].(string); ok {
        return v
    }
    return ""
}

func (c messageCarrier) Set(key, value string) {
    if c.msg.Attributes == nil {
        c.msg.Attributes = make(map[string]any)
    }
    c.msg.Attributes[key] = value
}

func (c messageCarrier) Keys() []string {
    keys := make([]string, 0, len(c.msg.Attributes))
    for k := range c.msg.Attributes {
        keys = append(keys, k)
    }
    return keys
}
```

### Metrics Collector

```go
type Collector struct {
    processed metric.Int64Counter
    duration  metric.Float64Histogram
    inflight  metric.Int64UpDownCounter
    errors    metric.Int64Counter
}

func NewCollector(opts ...Option) (*Collector, error) {
    cfg := newConfig(opts...)
    meter := cfg.MeterProvider.Meter("github.com/fxsml/gopipe/message/otel")

    processed, err := meter.Int64Counter("messaging.messages.processed",
        metric.WithDescription("Total messages processed"),
        metric.WithUnit("{message}"),
    )
    if err != nil {
        return nil, err
    }

    duration, err := meter.Float64Histogram("messaging.messages.duration",
        metric.WithDescription("Message processing duration"),
        metric.WithUnit("ms"),
    )
    if err != nil {
        return nil, err
    }

    inflight, err := meter.Int64UpDownCounter("messaging.messages.inflight",
        metric.WithDescription("Messages currently being processed"),
        metric.WithUnit("{message}"),
    )
    if err != nil {
        return nil, err
    }

    errors, err := meter.Int64Counter("messaging.messages.errors",
        metric.WithDescription("Message processing errors"),
        metric.WithUnit("{message}"),
    )
    if err != nil {
        return nil, err
    }

    return &Collector{
        processed: processed,
        duration:  duration,
        inflight:  inflight,
        errors:    errors,
    }, nil
}

// Collect implements pipe/middleware.MetricsCollector.
func (c *Collector) Collect(m *middleware.Metrics) {
    ctx := context.Background()

    c.processed.Add(ctx, 1)
    c.duration.Record(ctx, float64(m.Duration.Milliseconds()))

    if m.Error != nil {
        c.errors.Add(ctx, 1)
    }
}

// RecordInFlight updates in-flight gauge.
func (c *Collector) RecordInFlight(delta int64) {
    c.inflight.Add(context.Background(), delta)
}
```

### MessageTracker Integration

Add callback hook to MessageTracker (Plan 0012):

```go
// MessageTracker with observer support.
type MessageTracker struct {
    inFlight atomic.Int64
    closed   atomic.Bool
    drained  chan struct{}
    once     sync.Once
    observer func(delta int64)  // New: optional observer
}

// SetObserver sets a callback for in-flight changes.
func (t *MessageTracker) SetObserver(fn func(delta int64)) {
    t.observer = fn
}

func (t *MessageTracker) Enter() {
    t.inFlight.Add(1)
    if t.observer != nil {
        t.observer(1)
    }
}

func (t *MessageTracker) Exit() {
    if t.inFlight.Add(-1) == 0 && t.closed.Load() {
        t.once.Do(func() { close(t.drained) })
    }
    if t.observer != nil {
        t.observer(-1)
    }
}
```

Engine exposes tracker for otel plugin:

```go
// Tracker returns the message tracker for metrics integration.
// Only available after NewEngine(), before or after Start().
func (e *Engine) Tracker() *pipe.MessageTracker {
    return e.tracker
}
```

### Engine Plugin

```go
func Plugin(opts ...Option) message.Plugin {
    return func(e *message.Engine) error {
        cfg := newConfig(opts...)

        // Register tracing middleware
        if err := e.Use(Middleware(opts...)); err != nil {
            return err
        }

        // Set up metrics collector
        collector, err := NewCollector(opts...)
        if err != nil {
            return err
        }

        // Connect to message tracker for in-flight metrics
        if cfg.RecordInFlight {
            tracker := e.Tracker()
            if tracker != nil {
                tracker.SetObserver(collector.RecordInFlight)
            }
        }

        return nil
    }
}
```

## Pipe Package Telemetry

### Existing Middleware

The `pipe/middleware/metrics.go` already provides:

```go
type MetricsCollector func(metrics *Metrics)

func MetricsMiddleware[In, Out any](collect MetricsCollector) Middleware[In, Out]
```

### Integration Option

Create `pipe/otel/` package for otel-specific collectors:

```go
package otel

// Collector wraps pipe metrics for OpenTelemetry.
type Collector struct {
    // meters...
}

// MetricsCollector returns a pipe middleware.MetricsCollector.
func (c *Collector) MetricsCollector() middleware.MetricsCollector {
    return func(m *middleware.Metrics) {
        c.processed.Add(context.Background(), 1)
        c.duration.Record(context.Background(), float64(m.Duration.Milliseconds()))
        if m.Error != nil {
            c.errors.Add(context.Background(), 1)
        }
    }
}
```

**Recommendation**: Focus on `message/otel` first. Pipe telemetry can reuse the same patterns later.

## Testing Strategy

### Unit Tests

```go
// middleware_test.go
func TestMiddleware_CreatesSpan(t *testing.T)
func TestMiddleware_PropagatesContext(t *testing.T)
func TestMiddleware_RecordsError(t *testing.T)
func TestMiddleware_NoopWithoutProvider(t *testing.T)

// metrics_test.go
func TestCollector_RecordsProcessed(t *testing.T)
func TestCollector_RecordsDuration(t *testing.T)
func TestCollector_RecordsErrors(t *testing.T)
func TestCollector_RecordsInFlight(t *testing.T)

// propagation_test.go
func TestMessageCarrier_Get(t *testing.T)
func TestMessageCarrier_Set(t *testing.T)
func TestMessageCarrier_Keys(t *testing.T)
```

### Integration Tests

```go
func TestPlugin_RegistersMiddleware(t *testing.T)
func TestPlugin_ConnectsToTracker(t *testing.T)
func TestEngine_TracesMessageFlow(t *testing.T)
func TestEngine_PropagatesAcrossHandlers(t *testing.T)
```

### Benchmarks

```go
func BenchmarkMiddleware_Overhead(b *testing.B)
    // Target: <5% overhead with noop provider
    // Target: <10% overhead with recording provider

func BenchmarkCollector_RecordMetrics(b *testing.B)
    // Measure metric recording overhead
```

## Implementation Order

```
Task 1: Package structure & config
    │
    ▼
Task 2: Message carrier (propagation)
    │
    ▼
Task 3: Tracing middleware
    │
    ├──────────────────┐
    ▼                  ▼
Task 4: Metrics     Task 5: MessageTracker
collector           observer hook
    │                  │
    └────────┬─────────┘
             ▼
Task 6: Engine plugin
             │
             ▼
Task 7: Tests & benchmarks
             │
             ▼
Task 8: Documentation
```

## Dependencies

New dependencies to add:

```go
require (
    go.opentelemetry.io/otel v1.24.0
    go.opentelemetry.io/otel/metric v1.24.0
    go.opentelemetry.io/otel/trace v1.24.0
)
```

**Note**: These are lightweight - the SDK is optional and only needed at runtime when a provider is configured.

## Analysis

### Goals Verification

| Goal | Status | Notes |
|------|--------|-------|
| Distributed tracing | ✅ | Span per message, context propagation |
| Metrics | ✅ | Counters, histograms, gauges |
| Zero overhead when not configured | ✅ | Uses noop providers by default |
| Idiomatic otel integration | ✅ | Follows SDK patterns, semantic conventions |
| MessageTracker reuse | ✅ | Observer hook for in-flight metrics |

### Convention Compliance

| Convention | Status | Notes |
|------------|--------|-------|
| Separate package for optional deps | ✅ | `message/otel/` keeps core dependency-free |
| Config struct for constructors | ✅ | `Config` with functional options |
| Middleware pattern | ✅ | Wraps `ProcessFunc` like existing middleware |
| Plugin pattern | ✅ | `Plugin()` returns `message.Plugin` |

### Comparison: pipe/middleware vs message/otel

| Aspect | pipe/middleware/metrics.go | message/otel |
|--------|---------------------------|--------------|
| Purpose | Generic metrics collection | OpenTelemetry-specific |
| Dependencies | None (callback pattern) | otel SDK |
| Tracing | No | Yes (spans, propagation) |
| Flexibility | User provides collector | Built-in otel integration |
| Reuse | Can feed into otel | Provides otel-native metrics |

The existing `MetricsCollector` pattern is complementary - users can still implement custom collectors. The otel package provides ready-to-use instrumentation.

## Acceptance Criteria

- [ ] `message/otel` package created with tracing middleware
- [ ] Metrics collector with semantic convention metrics
- [ ] Context propagation via message attributes
- [ ] MessageTracker observer hook added (Plan 0012)
- [ ] Engine.Tracker() exposed for plugin access
- [ ] Plugin for easy setup
- [ ] All tests pass
- [ ] Benchmarks show <10% overhead with recording provider
- [ ] Documentation complete
- [ ] CHANGELOG updated
