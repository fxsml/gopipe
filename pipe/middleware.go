package pipe

// MiddlewareFunc wraps a Processor to add additional behavior to processing
// and cancellation.
type MiddlewareFunc[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// WithMiddleware adds middleware to the processing pipeline.
// Can be used multiple times. Middleware is applied in reverse order:
// for middlewares A, B, C, the execution flow is A→B→C→process.
func WithMiddleware[In, Out any](middleware MiddlewareFunc[In, Out]) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.middleware = append(cfg.middleware, middleware)
	}
}

func applyMiddleware[In, Out any](
	proc Processor[In, Out],
	middleware ...MiddlewareFunc[In, Out],
) Processor[In, Out] {
	for i := len(middleware) - 1; i >= 0; i-- {
		proc = middleware[i](proc)
	}
	return proc
}
