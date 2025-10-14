package middleware

import "github.com/fxsml/gopipe"

// MiddlewareFunc wraps a Processor to add functionality before/after processing or during cancellation.
type MiddlewareFunc[In, Out any] func(gopipe.Processor[In, Out]) gopipe.Processor[In, Out]

// NewProcessor creates a Processor with the provided processing function and middleware chain.
// Middleware is applied in reverse order: for middlewares A, B, C, the execution flow is A→B→C→process.
//
// NOTE: Cancellation is expected to be handled by middleware (such as UseLogger or UseCancel).
func NewProcessor[In, Out any](process gopipe.ProcessFunc[In, Out], mw ...MiddlewareFunc[In, Out]) gopipe.Processor[In, Out] {
	proc := gopipe.NewProcessor(process, func(in In, err error) {})
	for i := len(mw) - 1; i >= 0; i-- {
		proc = mw[i](proc)
	}
	return proc
}
