package middleware

import (
	"context"
	"time"
)

// Timeout wraps a ProcessFunc with a per-invocation timeout.
// Each call gets a fresh timeout context derived from the parent context,
// respecting shutdown signals. Zero or negative duration disables the timeout.
func Timeout[In, Out any](d time.Duration) Middleware[In, Out] {
	return func(next ProcessFunc[In, Out]) ProcessFunc[In, Out] {
		if d <= 0 {
			return next
		}
		return func(ctx context.Context, in In) ([]Out, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, in)
		}
	}
}
