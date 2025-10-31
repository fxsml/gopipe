package gopipe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

type DelayFunc func() func() time.Duration

func ConstantDelay(d time.Duration) DelayFunc {
	return func() func() time.Duration {
		return func() time.Duration {
			return d
		}
	}
}

func ExponentialDelay(
	baseDelay time.Duration,
	factor float64,
	maxDelay time.Duration,
) DelayFunc {
	return func() func() time.Duration {
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

			fmt.Println("attempt", attempt, "delay", delay)
			return delay
		}
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
			retryConfig{
				maxAttempts: maxAttempts,
				delay:       delay,
				timeout:     timeout,
			},
		)
	}
}

type retryConfig struct {
	maxAttempts int
	delay       DelayFunc
	timeout     time.Duration
}

func useRetry[In, Out any](config retryConfig) MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				return retry(ctx, next.Process, in, config)
			},
			func(in In, err error) {
				next.Cancel(in, err)
			},
		)
	}
}

var (
	// ErrMaxAttemptsReached is returned when all retry attempts fail
	ErrMaxAttemptsReached = errors.New("retry")

	// ErrTimeoutReached is returned when the overall retry operation times out
	ErrTimeoutReached = errors.New("retry")
)

func retry[In, Out any](
	ctx context.Context,
	handle func(context.Context, In) ([]Out, error),
	in In,
	config retryConfig,
) ([]Out, error) {
	out, err := handle(ctx, in)
	if err == nil {
		return out, nil
	}

	delay := config.delay()
	var cancel context.CancelFunc
	var timeoutCtx context.Context
	if config.timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, config.timeout)
		defer cancel()
	} else {
		timeoutCtx = context.Background()
	}

	attempt := 1

	// Determine if we're using a fixed number of attempts or retrying until context cancelled
	infiniteRetry := config.maxAttempts <= 0

	for {
		// Exit condition for fixed attempts
		if !infiniteRetry && attempt >= config.maxAttempts {
			return nil, fmt.Errorf("%w: failed after %d attempts: %w",
				ErrMaxAttemptsReached, attempt, err)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", ctx.Err(), err)
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("%w: %v", ErrTimeoutReached, err)
		case <-time.After(delay()):
		}

		// Try executing the function
		out, err = handle(ctx, in)
		if err == nil {
			return out, nil
		}

		attempt++
	}
}
