package middleware

import (
	"context"
	"time"
)

// ContextConfig configures context behavior for processing.
type ContextConfig struct {
	// Timeout sets maximum duration for each process operation.
	// If zero, no timeout is applied.
	Timeout time.Duration

	// Background controls whether the parent context is propagated.
	// If true, each process call receives a fresh background context,
	// isolating processing from pipeline cancellation.
	Background bool

	// ReturnWhenDone causes the middleware to return ctx.Err() immediately
	// if the context is already canceled before processing starts.
	// This allows the pipeline to drain quickly when canceled.
	ReturnWhenDone bool
}

// Context returns middleware that manages context for each process call.
// It controls timeout, context propagation, and early return on cancellation.
//
// When Background is true, the processor receives a background context,
// preventing pipeline cancellation from interrupting in-flight work.
// When Timeout is set, each call is bounded by the specified duration.
// When ReturnWhenDone is true, processing is skipped if context is already canceled.
func Context[In, Out any](cfg ContextConfig) Middleware[In, Out] {
	return func(next ProcessFunc[In, Out]) ProcessFunc[In, Out] {
		return func(ctx context.Context, in In) ([]Out, error) {
			if cfg.ReturnWhenDone && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if cfg.Background {
				ctx = context.Background()
			}
			if cfg.Timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
				defer cancel()
			}
			return next(ctx, in)
		}
	}
}
