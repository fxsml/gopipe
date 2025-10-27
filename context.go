package gopipe

import (
	"context"
	"time"
)

func UseContext[In, Out any](timeout time.Duration, contextPropagation bool) MiddlewareFunc[In, Out] {
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
