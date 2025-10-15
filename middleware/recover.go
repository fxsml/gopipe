package middleware

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/fxsml/gopipe"
)

// RecoveryError wraps a panic value with the stack trace.
type RecoveryError struct {
	PanicValue any
	StackTrace string
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("panic recovered: %v", e.PanicValue)
}

// UseRecover creates middleware that recovers from panics in the processing function.
// When a panic occurs, it converts the panic to an error. The stack trace is captured
// and included in the RecoveryError. The stack trace is also printed to stderr in the
// CancelFunc.
func UseRecover[In, Out any]() MiddlewareFunc[In, Out] {
	return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
		return gopipe.NewProcessor(
			func(ctx context.Context, in In) (out Out, err error) {
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
