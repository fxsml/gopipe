package gopipe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

type RetryDelayFunc func() func() time.Duration

func ConstantRetryDelay(d time.Duration) RetryDelayFunc {
	return func() func() time.Duration {
		return func() time.Duration {
			jitter := 1.0 + (rand.Float64()*0.4 - 0.2) // between 0.8 and 1.2
			return time.Duration(float64(d) * jitter)
		}
	}
}

func ExponentialRetryDelay(
	baseDelay time.Duration,
	factor float64,
	maxDelay time.Duration,
) RetryDelayFunc {
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

			return delay
		}
	}
}

type IsRetryableFunc func(error) bool

func AlwaysRetry(errs ...error) IsRetryableFunc {
	if len(errs) == 0 {
		return func(err error) bool {
			return true
		}
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

func NeverRetry(errs ...error) IsRetryableFunc {
	if len(errs) == 0 {
		return func(err error) bool {
			return false
		}
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

// WithRetryConfig adds retry middleware to the processing pipeline.
// maxAttempts specifies the maximum number of attempts (including the initial attempt).
// If maxAttempts is set to 0 or negative, the operation will retry indefinitely until the context is cancelled.
// delay specifies the delay function between retries.
// timeout specifies the overall timeout for all retry attempts combined. If zero, no timeout is applied.
func WithRetryConfig[In, Out any](retryConfig *RetryConfig) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.retry = useRetry[In, Out](retryConfig)
	}
}

var defaultRetryConfig = RetryConfig{
	IsRetryable: AlwaysRetry(),
	Delay:       ConstantRetryDelay(1 * time.Second),
	MaxAttempts: 2,
}

type RetryConfig struct {
	IsRetryable IsRetryableFunc
	Delay       RetryDelayFunc
	MaxAttempts int
	Timeout     time.Duration
}

func (c *RetryConfig) parse() *RetryConfig {
	if c == nil {
		return nil
	}
	if c.IsRetryable == nil {
		c.IsRetryable = defaultRetryConfig.IsRetryable
	}
	if c.Delay == nil {
		c.Delay = defaultRetryConfig.Delay
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultRetryConfig.MaxAttempts
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultRetryConfig.Timeout
	}
	return c
}

func useRetry[In, Out any](config *RetryConfig) MiddlewareFunc[In, Out] {
	if config = config.parse(); config == nil {
		return nil
	}
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				state := newRetryState(config.Timeout, config.MaxAttempts)
				delay := config.Delay()

				for {
					out, err := next.Process(state.Context(ctx), in)
					if err == nil {
						return out, nil
					}
					state.appendCause(err)
					if !config.IsRetryable(err) {
						return nil, state.error(ErrNotRetryable)
					}

					if config.MaxAttempts > 0 && state.Attempts >= config.MaxAttempts {
						return nil, state.error(ErrRetryMaxAttempts)
					}

					select {
					case <-ctx.Done():
						return nil, state.error(ctx.Err())
					case <-time.After(config.Timeout - time.Since(state.Start)):
						return nil, state.error(ErrRetryTimeout)
					case <-time.After(delay()):
					}
				}
			},
			func(in In, err error) {
				next.Cancel(in, err)
			},
		)
	}
}

var (
	ErrRetry = errors.New("retry")

	// ErrRetryMaxAttempts is returned when all retry attempts fail
	ErrRetryMaxAttempts = fmt.Errorf("max %w attempts reached", ErrRetry)

	// ErrRetryTimeout is returned when the overall retry operation times out
	ErrRetryTimeout = fmt.Errorf("%w timeout reached", ErrRetry)

	// ErrNotRetryable is returned when an error is not retryable
	ErrNotRetryable = fmt.Errorf("not %wable", ErrRetry)
)

type RetryState struct {
	Timeout     time.Duration
	MaxAttempts int

	Start    time.Time
	Attempts int
	Duration time.Duration
	Causes   []error
	Err      error
}

func newRetryState(
	timeout time.Duration,
	maxAttempts int,
) *RetryState {
	return &RetryState{
		Timeout:     timeout,
		MaxAttempts: maxAttempts,
		Start:       time.Now(),
		Attempts:    0,
	}
}

func (s *RetryState) appendCause(err error) {
	s.Attempts++
	s.Duration = time.Since(s.Start)
	s.Causes = append(s.Causes, err)
}

func (s *RetryState) error(err error) error {
	s.Duration = time.Since(s.Start)
	s.Err = err
	return &retryStateErrorWrapper{state: s}
}

func (s *RetryState) Cause() error {
	if s == nil || len(s.Causes) == 0 {
		return nil
	}
	return s.Causes[len(s.Causes)-1]
}

type retryStateKeyType struct{}

var retryStateKey = retryStateKeyType{}

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

func (s *RetryState) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, retryStateKey, s)
}

type retryStateErrorWrapper struct {
	state *RetryState
}

func (w *retryStateErrorWrapper) Error() string {
	if w.state == nil || len(w.state.Causes) == 0 {
		return fmt.Sprintf("gopipe: %s", ErrRetry.Error())
	}
	return fmt.Sprintf("gopipe: %s: %s", w.state.Err, w.state.Causes[len(w.state.Causes)-1])
}

func (w *retryStateErrorWrapper) Unwrap() []error {
	// TODO: should w.state.Err be the first element?
	return append(w.state.Causes, w.state.Err)
}
