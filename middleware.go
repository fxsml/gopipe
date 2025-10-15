package gopipe

// MiddlewareFunc wraps a Processor to add additional behavior to processing
// and cancellation.
type MiddlewareFunc[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// ApplyMiddleware applies a chain of middleware to a Processor.
// Middleware is applied in reverse order: for middlewares A, B, C,
// the execution flow is A→B→C→process.
func ApplyMiddleware[In, Out any](
	proc Processor[In, Out],
	middleware ...MiddlewareFunc[In, Out],
) Processor[In, Out] {
	for i := len(middleware) - 1; i >= 0; i-- {
		proc = middleware[i](proc)
	}
	return proc
}
