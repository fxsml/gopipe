package middleware

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/fxsml/gopipe/message"
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

// Recover wraps message processing with panic recovery.
// Any panic that occurs during processing is caught and converted
// into a RecoveryError with the stack trace captured.
func Recover() message.Middleware {
	return func(next message.ProcessFunc) message.ProcessFunc {
		return func(ctx context.Context, msg *message.Message) (out []*message.Message, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = &RecoveryError{
						PanicValue: r,
						StackTrace: string(debug.Stack()),
					}
				}
			}()
			return next(ctx, msg)
		}
	}
}
