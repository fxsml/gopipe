package gopipe

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/fxsml/gopipe/internal/retry"
)

type DelayFunc func() time.Duration

func ConstantDelay(d time.Duration) DelayFunc {
	return func() time.Duration {
		return d
	}
}

func ExponentialDelay(
	baseDelay time.Duration,
	factor float64,
	maxDelay time.Duration,
) DelayFunc {
	attempt := 0
	return func() time.Duration {
		attempt++

		// Calculate exponential delay: baseDelay * factor^(attempt-1)
		delaySeconds := float64(baseDelay) * math.Pow(factor, float64(attempt-1))

		// Apply jitter
		jitterFactor := 1.0 + (rand.Float64()*0.4 - 0.2) // between 0.8 and 1.2
		delaySeconds *= jitterFactor

		delay := time.Duration(delaySeconds)

		// Apply maximum if specified and exceeded
		if maxDelay > 0 && delay > maxDelay {
			delay = maxDelay
		}

		return delay
	}
}

// WithRetry adds retry middleware to the processing pipeline.
// maxAttempts specifies the maximum number of attempts (including the initial attempt).
// If maxAttempts is set to 0 or negative, the operation will retry indefinitely until the context is cancelled.
// delay specifies the delay function between retries.
// timeout specifies the overall timeout for all retry attempts combined. If zero, no timeout is applied.
func WithRetry[In, Out any](
	maxAttempts int,
	delay DelayFunc,
	timeout time.Duration,
) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.retry = useRetry[In, Out](
			retry.WithMaxAttempts(maxAttempts),
			retry.WithDelay(delay),
			retry.WithTimeout(timeout),
		)
	}
}

func useRetry[In, Out any](opts ...retry.Option) MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				var out []Out
				var err error
				handle := func(ctx context.Context) error {
					out, err = next.Process(ctx, in)
					return err
				}
				return out, retry.Retry(ctx, handle, opts...)
			},
			func(in In, err error) {
				next.Cancel(in, err)
			},
		)
	}
}
