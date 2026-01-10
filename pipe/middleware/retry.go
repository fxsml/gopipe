package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

var (
	// ErrRetry is the base error for retry operations.
	ErrRetry = errors.New("gopipe retry")

	// ErrRetryMaxAttempts is returned when all retry attempts fail.
	ErrRetryMaxAttempts = fmt.Errorf("%w: max attempts reached", ErrRetry)

	// ErrRetryTimeout is returned when the overall retry operation times out.
	ErrRetryTimeout = fmt.Errorf("%w: timeout reached", ErrRetry)

	// ErrRetryNotRetryable is returned when an error is not retryable.
	ErrRetryNotRetryable = fmt.Errorf("%w: not retryable", ErrRetry)
)

// BackoffFunc returns the wait duration for a retry attempt.
// The attempt parameter is one-based (1 for first retry, 2 for second, etc.).
type BackoffFunc func(attempt int) time.Duration

// ConstantBackoff creates a backoff function that returns a constant duration with optional jitter.
// The delay parameter specifies the base wait time for all retry attempts.
// The jitter parameter controls randomization: 0.0 = no jitter, 0.2 = ±20% variation.
func ConstantBackoff(delay time.Duration, jitter float64) BackoffFunc {
	applyJitter := newApplyJitterFunc(jitter)
	return func(attempt int) time.Duration {
		return applyJitter(delay)
	}
}

// ExponentialBackoff creates a backoff function with exponential backoff and jitter.
// Each retry attempt uses baseDelay * factor^(attempt-1) with random jitter applied.
// The maxDelay parameter caps the maximum backoff duration (0 = no limit).
func ExponentialBackoff(initialDelay time.Duration, factor float64, maxDelay time.Duration, jitter float64) BackoffFunc {
	applyJitter := newApplyJitterFunc(jitter)
	return func(attempt int) time.Duration {
		backoff := time.Duration(float64(initialDelay) * math.Pow(factor, float64(attempt-1)))
		if maxDelay > 0 && backoff > maxDelay {
			backoff = maxDelay
		}
		return applyJitter(backoff)
	}
}

// ShouldRetryFunc determines whether an error should trigger a retry attempt.
type ShouldRetryFunc func(error) bool

// ShouldRetry creates a function that retries on specific errors.
// If no errors are specified, all errors trigger retries.
func ShouldRetry(errs ...error) ShouldRetryFunc {
	if len(errs) == 0 {
		return func(err error) bool { return true }
	}
	return func(err error) bool {
		for _, e := range errs {
			if errors.Is(err, e) {
				return true
			}
		}
		return false
	}
}

// ShouldNotRetry creates a function that skips retries on specific errors.
// If no errors are specified, no errors trigger retries.
func ShouldNotRetry(errs ...error) ShouldRetryFunc {
	if len(errs) == 0 {
		return func(err error) bool { return false }
	}
	return func(err error) bool {
		for _, e := range errs {
			if errors.Is(err, e) {
				return false
			}
		}
		return true
	}
}

// RetryConfig configures retry behavior for failed operations.
type RetryConfig struct {
	// ShouldRetry determines which errors trigger retry attempts.
	// If nil, defaults to retrying all errors.
	ShouldRetry ShouldRetryFunc

	// Backoff produces the wait duration between retry attempts.
	// If nil, defaults to 1 second constant backoff with jitter ±20%.
	Backoff BackoffFunc

	// MaxAttempts limits the total number of processing attempts, including the initial attempt.
	// Default is 3 attempts. Negative values allow unlimited retries.
	MaxAttempts int

	// Timeout sets the overall time limit for all processing attempts combined.
	// Zero or negative value means no timeout. Default is 1 minute.
	Timeout time.Duration
}

// RetryState tracks the progress and history of retry attempts.
type RetryState struct {
	// Timeout is the configured overall timeout for all attempts.
	Timeout time.Duration
	// MaxAttempts is the configured maximum number of attempts.
	MaxAttempts int
	// Start is the time when the first attempt started.
	Start time.Time
	// Attempts is the total number of processing attempts made (1-based).
	Attempts int
	// Duration is the total elapsed time since Start.
	Duration time.Duration
	// Causes is a list of all errors encountered during attempts.
	Causes []error
	// Err is the error that caused the retry process to abort (final error).
	Err error
}

// RetryStateFromContext extracts the RetryState from a context.
// Returns nil if no RetryState is present.
func RetryStateFromContext(ctx context.Context) *RetryState {
	if ctx == nil {
		return nil
	}
	if state, ok := ctx.Value(retryStateKey).(*RetryState); ok {
		return state
	}
	return nil
}

// RetryStateFromError extracts the RetryState from an error.
// Returns nil if no RetryState is present.
func RetryStateFromError(err error) *RetryState {
	if err == nil {
		return nil
	}
	var w *retryStateErrorWrapper
	if errors.As(err, &w) {
		return w.state
	}
	return nil
}

var defaultRetryConfig = RetryConfig{
	ShouldRetry: ShouldRetry(),
	Backoff:     ConstantBackoff(1*time.Second, 0.2),
	MaxAttempts: 3,
	Timeout:     1 * time.Minute,
}

func (c RetryConfig) parse() RetryConfig {
	if c.ShouldRetry == nil {
		c.ShouldRetry = defaultRetryConfig.ShouldRetry
	}
	if c.Backoff == nil {
		c.Backoff = defaultRetryConfig.Backoff
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = defaultRetryConfig.MaxAttempts
	} else if c.MaxAttempts < 0 {
		c.MaxAttempts = 0
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultRetryConfig.Timeout
	}
	return c
}

func newRetryState(timeout time.Duration, maxAttempts int) *RetryState {
	return &RetryState{
		Timeout:     timeout,
		MaxAttempts: maxAttempts,
		Start:       time.Now(),
		Attempts:    0,
	}
}

func (s *RetryState) appendCause(err error) {
	s.Duration = time.Since(s.Start)
	s.Causes = append(s.Causes, err)
}

func (s *RetryState) error(err error) error {
	s.Duration = time.Since(s.Start)
	s.Err = err
	return &retryStateErrorWrapper{state: s}
}

func (s *RetryState) context(ctx context.Context) context.Context {
	s.Attempts++
	return context.WithValue(ctx, retryStateKey, s)
}

type retryStateKeyType struct{}

var retryStateKey = retryStateKeyType{}

type retryStateErrorWrapper struct {
	state *RetryState
}

func (w *retryStateErrorWrapper) Error() string {
	if w.state == nil || len(w.state.Causes) == 0 {
		return ErrRetry.Error()
	}
	return fmt.Sprintf("%s: %s", w.state.Err, w.state.Causes[len(w.state.Causes)-1])
}

func (w *retryStateErrorWrapper) Unwrap() []error {
	return append([]error{w.state.Err}, w.state.Causes...)
}

func newApplyJitterFunc(jitter float64) func(d time.Duration) time.Duration {
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	return func(d time.Duration) time.Duration {
		jitterFactor := 1.0 + (rand.Float64()*2*jitter - jitter)
		return time.Duration(float64(d) * jitterFactor)
	}
}

// Retry wraps a ProcessFunc with retry logic.
// Failed operations are retried based on ShouldRetry logic, with Backoff between attempts,
// until MaxAttempts is reached or Timeout expires.
func Retry[In, Out any](cfg RetryConfig) Middleware[In, Out] {
	cfg = cfg.parse()
	return func(next ProcessFunc[In, Out]) ProcessFunc[In, Out] {
		return func(ctx context.Context, in In) ([]Out, error) {
			state := newRetryState(cfg.Timeout, cfg.MaxAttempts)

			for {
				out, err := next(state.context(ctx), in)
				if err == nil {
					return out, nil
				}
				state.appendCause(err)
				if !cfg.ShouldRetry(err) {
					return nil, state.error(ErrRetryNotRetryable)
				}

				if cfg.MaxAttempts > 0 && state.Attempts >= cfg.MaxAttempts {
					return nil, state.error(ErrRetryMaxAttempts)
				}

				var timeoutCh <-chan time.Time
				if cfg.Timeout > 0 {
					remaining := cfg.Timeout - time.Since(state.Start)
					if remaining <= 0 {
						return nil, state.error(ErrRetryTimeout)
					}
					timeoutCh = time.After(remaining)
				}

				select {
				case <-ctx.Done():
					return nil, state.error(ctx.Err())
				case <-timeoutCh:
					return nil, state.error(ErrRetryTimeout)
				case <-time.After(cfg.Backoff(state.Attempts)):
				}
			}
		}
	}
}
