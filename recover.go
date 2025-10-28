package gopipe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
)

// RecoveryError wraps a panic value with the stack trace.
// This allows panics to be converted to regular errors and handled gracefully.
type RecoveryError struct {
	// PanicValue is the original value that was passed to panic().
	PanicValue any
	// StackTrace contains the full stack trace at the point of panic.
	StackTrace string
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("panic recovered: %v", e.PanicValue)
}

// WithRecover enables panic recovery in process functions. When enabled, any panic
// that occurs during processing is caught and converted into a RecoveryError.
// The stack trace is captured and included in the RecoveryError. The stack trace
// is also printed to stderr in the CancelFunc.
func WithRecover[In, Out any]() Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.recover = true
	}
}

func useRecover[In, Out any]() MiddlewareFunc[In, Out] {
	return func(next Processor[In, Out]) Processor[In, Out] {
		return NewProcessor(
			func(ctx context.Context, in In) (out []Out, err error) {
				defer func() {
					if r := recover(); r != nil {
						err = &RecoveryError{
							PanicValue: r,
							StackTrace: string(debug.Stack()),
						}
					}
				}()

				return next.Process(ctx, in)
			},
			func(in In, err error) {
				next.Cancel(in, err)
				var recErr *RecoveryError
				if errors.As(err, &recErr) {
					fmt.Fprint(os.Stderr, err.Error(), "\n", recErr.StackTrace)
				}
			},
		)
	}
}
