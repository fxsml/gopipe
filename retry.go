package gopipe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryOption represents a functional option for configuring retry behavior
type RetryOption func(*RetryConfig)

// DelayFunc defines a function that returns a time.Duration for delays between retries
type DelayFunc func() time.Duration

// RetryConfig holds all the configurable retry options
type RetryConfig struct {
	MaxAttempts int // if <= 0, will retry until context is cancelled
	Delay       DelayFunc
	Timeout     time.Duration
}

var (
	// ErrMaxAttemptsReached is returned when all retry attempts fail
	ErrMaxAttemptsReached = errors.New("retry")

	// ErrTimeoutReached is returned when the overall retry operation times out
	ErrTimeoutReached = errors.New("retry")
)

// WithDelay sets a custom delay function
func WithDelay(delay func() time.Duration) RetryOption {
	return func(o *RetryConfig) {
		if delay != nil {
			o.Delay = delay
		}
	}
}

// ConstantDelay returns a DelayFunc that always returns the same delay duration
func ConstantDelay(delay time.Duration) DelayFunc {
	return func() time.Duration {
		return delay
	}
}

// WithExponentialBackoff sets an exponential backoff delay strategy
func WithExponentialBackoff(baseDelay time.Duration, factor float64, jitter bool, maxDelay time.Duration) RetryOption {
	attempt := 0
	return WithDelay(func() time.Duration {
		attempt++

		// Calculate exponential delay: baseDelay * factor^(attempt-1)
		delaySeconds := float64(baseDelay) * math.Pow(factor, float64(attempt-1))

		// Apply jitter if requested (±20%)
		if jitter {
			jitterFactor := 1.0 + (rand.Float64()*0.4 - 0.2) // between 0.8 and 1.2
			delaySeconds *= jitterFactor
		}

		delay := time.Duration(delaySeconds)

		// Apply maximum if specified and exceeded
		if maxDelay > 0 && delay > maxDelay {
			delay = maxDelay
		}

		return delay
	})
}

// WithDefaultBackoff returns a delay function with defaults for exponential backoff:
// - 100ms base delay
// - Doubles with each attempt (factor of 2.0)
// - Includes jitter to prevent synchronized retries
// - Maximum delay of 30 seconds
func WithDefaultBackoff() RetryOption {
	return WithExponentialBackoff(
		100*time.Millisecond,
		2.0,
		true,
		30*time.Second,
	)
}

// WithRetryTimeout sets an overall timeout for the entire retry operation
func WithRetryTimeout(timeout time.Duration) RetryOption {
	return func(o *RetryConfig) {
		if timeout > 0 {
			o.Timeout = timeout
		}
	}
}

func UseRetry[In, Out any](config RetryConfig) MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				var out []Out
				handle := func(ctx context.Context) error {
					var err error
					out, err = next.Process(ctx, in)
					return err
				}
				return out, Retry(ctx, handle, config)
			},
			func(in In, err error) {
				next.Cancel(in, err)
			},
		)
	}
}

// Retry attempts to execute the function fn using the specified options.
// If no options are provided, default values will be used.
// By default, it will retry indefinitely until the context is cancelled.
func Retry(
	ctx context.Context,
	handle func(ctx context.Context) error,
	config RetryConfig,
) error {
	// Try executing the function
	err := handle(ctx)
	if err == nil {
		return nil
	}

	// Apply timeout if specified
	var cancel context.CancelFunc
	var timeoutCtx context.Context
	if config.Timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	} else {
		timeoutCtx = context.Background()
	}

	attempt := 1

	// Determine if we're using a fixed number of attempts or retrying until context cancelled
	infiniteRetry := config.MaxAttempts <= 0

	for {
		// Exit condition for fixed attempts
		if !infiniteRetry && attempt >= config.MaxAttempts {
			return fmt.Errorf("%w: failed after %d attempts: %w",
				ErrMaxAttemptsReached, attempt, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ctx.Err(), err)
		case <-timeoutCtx.Done():
			return fmt.Errorf("%w: %v", ErrTimeoutReached, err)
		case <-time.After(config.Delay()):
		}

		// Try executing the function
		if err = handle(ctx); err == nil {
			return nil // Success
		}

		attempt++
	}
}
