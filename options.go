package gopipe

import (
	"context"
	"time"
)

type config[In, Out any] struct {
	concurrency        int
	buffer             int
	timeout            time.Duration
	contextPropagation bool
	cancel             CancelFunc[In]
	middleware         []MiddlewareFunc[In, Out]
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

func (p *config[In, Out]) newProcessCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if !p.contextPropagation {
		ctx = context.Background()
	}
	if p.timeout > 0 {
		return context.WithTimeout(ctx, p.timeout)
	}
	return context.WithCancel(ctx)
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
func WithMiddleware[In, Out any](mw MiddlewareFunc[In, Out]) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.middleware = append(cfg.middleware, mw)
	}
}
