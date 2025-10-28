package gopipe

import (
	"context"
	"time"
)

// WithTimeout sets maximum duration for each process operation.
// If the processing exceeds the timeout, the context will be cancelled.
func WithTimeout[In, Out any](timeout time.Duration) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		if timeout > 0 {
			cfg.timeout = timeout
		}
	}
}

// WithoutContextPropagation disables passing parent context to process functions.
// Each process call will receive a background context instead.
func WithoutContextPropagation[In, Out any]() Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.contextPropagation = false
	}
}

func useContext[In, Out any](timeout time.Duration, contextPropagation bool) MiddlewareFunc[In, Out] {
	newCtx := func(ctx context.Context) (context.Context, context.CancelFunc) {
		if !contextPropagation {
			ctx = context.Background()
		}
		if timeout > 0 {
			return context.WithTimeout(ctx, timeout)
		}
		return context.WithCancel(ctx)
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				ctx, cancel := newCtx(ctx)
				defer cancel()
				return next.Process(ctx, in)
			},
			func(in In, err error) {
				next.Cancel(in, err)
			},
		)
	}
}
