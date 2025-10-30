package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// Option represents a functional option for configuring retry behavior
type Option func(*options)

// options holds all the configurable retry options
type options struct {
	maxAttempts int // if <= 0, will retry until context is cancelled
	delay       func() time.Duration
	timeout     time.Duration
}

var (
	// ErrMaxAttemptsReached is returned when all retry attempts fail
	ErrMaxAttemptsReached = errors.New("retry")

	// ErrTimeoutReached is returned when the overall retry operation times out
	ErrTimeoutReached = errors.New("retry")
)

// WithMaxAttempts sets the maximum number of retry attempts.
// If maxAttempts is set to 0 or negative, the operation will retry indefinitely
// until the context is cancelled.
func WithMaxAttempts(maxAttempts int) Option {
	return func(o *options) {
		o.maxAttempts = maxAttempts
	}
}

// WithDelay sets a custom delay function
func WithDelay(delay func() time.Duration) Option {
	return func(o *options) {
		if delay != nil {
			o.delay = delay
		}
	}
}

// WithConstantDelay sets a constant delay between retries
func WithConstantDelay(delay time.Duration) Option {
	return WithDelay(func() time.Duration {
		return delay
	})
}

// WithExponentialBackoff sets an exponential backoff delay strategy
func WithExponentialBackoff(baseDelay time.Duration, factor float64, jitter bool, maxDelay time.Duration) Option {
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
func WithDefaultBackoff() Option {
	return WithExponentialBackoff(
		100*time.Millisecond,
		2.0,
		true,
		30*time.Second,
	)
}

// WithTimeout sets an overall timeout for the entire retry operation
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// RetryFunc represents a function that can be retried
type RetryFunc func(ctx context.Context) error

// Retry attempts to execute the function fn using the specified options.
// If no options are provided, default values will be used.
// By default, it will retry indefinitely until the context is cancelled.
func Retry(ctx context.Context, fn RetryFunc, opts ...Option) error {
	// Try executing the function
	err := fn(ctx)
	if err == nil {
		return nil
	}

	// Default options - retry indefinitely until context is cancelled
	config := &options{}
	WithDefaultBackoff()(config)

	// Apply provided options
	for _, opt := range opts {
		opt(config)
	}

	// Apply timeout if specified
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
			return fmt.Errorf("%w: failed after %d attempts: %w",
				ErrMaxAttemptsReached, attempt, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ctx.Err(), err)
		case <-timeoutCtx.Done():
			return fmt.Errorf("%w: %v", ErrTimeoutReached, err)
		case <-time.After(config.delay()):
		}

		// Try executing the function
		if err = fn(ctx); err == nil {
			return nil // Success
		}

		attempt++
	}
}

// MustRetry is similar to Retry but panics if the retry operation fails.
// This is useful for operations that must succeed and where error handling is not desired.
func MustRetry(ctx context.Context, fn RetryFunc, opts ...Option) {
	if err := Retry(ctx, fn, opts...); err != nil {
		panic(err)
	}
}
