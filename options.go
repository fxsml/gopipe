package gopipe

import (
	"time"
)

type config[In, Out any] struct {
	concurrency        int
	buffer             int
	timeout            time.Duration
	contextPropagation bool
	cancel             CancelFunc[In]

	middleware       []MiddlewareFunc[In, Out]
	metricsCollector []MetricsCollector
	metadataProvider []MiddlewareFunc[In, Out]
	recover          bool
	loggerConfig     *LoggerConfig
}

func parseConfig[In, Out any](opts []Option[In, Out]) config[In, Out] {
	c := config[In, Out]{
		concurrency:        1,
		buffer:             0,
		timeout:            0,
		contextPropagation: true,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

func (c *config[In, Out]) applyMiddleware(proc Processor[In, Out]) Processor[In, Out] {
	if c.cancel != nil {
		proc = NewProcessor(proc.Process, c.cancel)
	}

	if c.timeout > 0 || !c.contextPropagation {
		proc = UseContext[In, Out](c.timeout, c.contextPropagation)(proc)
	}

	if logger := NewMetricsLogger(c.loggerConfig); logger != nil {
		c.metricsCollector = append(c.metricsCollector, logger)
	}

	if len(c.metricsCollector) > 1 {
		proc = UseMetrics[In, Out](NewMetricsDistributor(c.metricsCollector...))(proc)
	}
	if len(c.metricsCollector) == 1 {
		proc = UseMetrics[In, Out](c.metricsCollector[0])(proc)
	}

	proc = ApplyMiddleware(proc, c.middleware...)
	proc = ApplyMiddleware(proc, c.metadataProvider...)

	if c.recover {
		proc = UseRecover[In, Out]()(proc)
	}

	return proc
}

// Option configures behavior of a Pipe.
type Option[In, Out any] func(*config[In, Out])

// WithConcurrency sets worker count for concurrent processing.
func WithConcurrency[In, Out any](concurrency int) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if concurrency > 0 {
			cfg.concurrency = concurrency
		}
	}
}

// WithBuffer sets output channel buffer size.
func WithBuffer[In, Out any](buffer int) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if buffer > 0 {
			cfg.buffer = buffer
		}
	}
}

// WithTimeout sets maximum duration for each process operation.
func WithTimeout[In, Out any](timeout time.Duration) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if timeout > 0 {
			cfg.timeout = timeout
		}
	}
}

// WithoutContextPropagation disables passing parent context to process functions.
func WithoutContextPropagation[In, Out any]() Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.contextPropagation = false
	}
}

// WithCancel provides a cancel function to the processor.
// If set, this overrides any existing cancel function.
func WithCancel[In, Out any](cancel func(In, error)) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.cancel = cancel
	}
}

// WithMiddleware adds middleware to the processing pipeline.
// Can be used multiple times. Middleware is applied in reverse order:
// for middlewares A, B, C, the execution flow is A→B→C→process.
func WithMiddleware[In, Out any](middleware MiddlewareFunc[In, Out]) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.middleware = append(cfg.middleware, middleware)
	}
}

func WithMetrics[In, Out any](collector MetricsCollector) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.metricsCollector = append(cfg.metricsCollector, collector)
	}
}

func WithMetadata[In, Out any](provider MetadataProvider[In]) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.metadataProvider = append(cfg.metadataProvider, UseMetadata[In, Out](provider))
	}
}

func WithRecover[In, Out any]() Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.recover = true
	}
}

func WithLoggerConfig[In, Out any](loggerConfig *LoggerConfig) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.loggerConfig = loggerConfig
	}
}
