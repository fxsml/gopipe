package gopipe

import (
	"context"
	"time"
)

type config struct {
	concurrency        int
	buffer             int
	timeout            time.Duration
	contextPropagation bool
	// metrics collects optional runtime metrics. Nil means disabled.
	metrics Metrics
}

func defaultConfig() config {
	return config{
		concurrency:        1,
		buffer:             0,
		timeout:            0,
		contextPropagation: true,
		metrics:            nil,
	}
}

// Option configures behavior of pipeline processing stages.
type Option func(*config)

func (p *config) newProcessCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if !p.contextPropagation {
		ctx = context.Background()
	}
	if p.timeout > 0 {
		return context.WithTimeout(ctx, p.timeout)
	}
	return context.WithCancel(ctx)
}

// WithConcurrency sets worker count for concurrent processing.
func WithConcurrency(concurrency int) Option {
	return func(cfg *config) {
		if concurrency > 0 {
			cfg.concurrency = concurrency
		}
	}
}

// WithBuffer sets output channel buffer size.
func WithBuffer(buffer int) Option {
	return func(cfg *config) {
		if buffer > 0 {
			cfg.buffer = buffer
		}
	}
}

// WithTimeout sets maximum duration for each process operation.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *config) {
		if timeout > 0 {
			cfg.timeout = timeout
		}
	}
}

// WithoutContextPropagation disables passing parent context to process functions.
func WithoutContextPropagation() Option {
	return func(cfg *config) {
		cfg.contextPropagation = false
	}
}

// Metrics defines a minimal surface for collecting pipeline metrics.
// Implementations may forward these to Prometheus, OpenTelemetry, etc.
type Metrics interface {
	IncSuccess()
	IncFailure()
	IncCancelled()
	IncInFlight()
	DecInFlight()
	ObserveProcessingDuration(d time.Duration)
	ObserveBufferSize(n int)
	ObserveBatchSize(n int)
}

// WithMetrics attaches a Metrics implementation to the pipeline. When set,
// various stages will emit metric events.
func WithMetrics(m Metrics) Option {
	return func(cfg *config) {
		cfg.metrics = m
	}
}
