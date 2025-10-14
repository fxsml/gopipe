## ADR-9: Middleware Pattern

See #9

### Context

With the introduction of the Processor abstraction, gopipe needs a standardized way to extend processor functionality with cross-cutting concerns like logging, metrics, and error handling. Currently, such functionality must be implemented separately for each processor, leading to code duplication and inconsistent implementations.

### Decision

Introduce a middleware pattern using the following approach:

```go
type MiddlewareFunc[In, Out any] func(gopipe.Processor[In, Out]) gopipe.Processor[In, Out]

func NewProcessor[In, Out any](process gopipe.ProcessFunc[In, Out], mw ...MiddlewareFunc[In, Out]) gopipe.Processor[In, Out] {
	proc := gopipe.NewProcessor(process, func(in In, err error) {})
	for i := len(mw) - 1; i >= 0; i-- {
		proc = mw[i](proc)
	}
	return proc
}
```

Middlewares are applied in reverse order (from last to first), creating a wrapping pattern where the first middleware in the list is the outermost wrapper. This ensures that for a given list of middlewares A, B, C:
1. The Process flow will be A → B → C → base processor
2. The Cancel flow will also be A → B → C

To provide a CancelFunc to the processor, a middleware with the following signature is introduced:

```go
type UseCancel[In, Out any] func(cancel gopipe.CancelFunc[In]) MiddlewareFunc[In, Out]
```

### Consequences

**Positive**
- Enables clean separation of cross-cutting concerns from core processing logic
- Provides a consistent pattern for extending processor behavior
- Facilitates reuse of common functionality across different processors
- Improves maintainability by keeping extensions modular and composable
- Allows for conditional application of middleware based on runtime requirements

**Negative**
- Introduces complexity in understanding the execution flow, particularly the reverse order application
- May add slight performance overhead due to additional function calls
- Requires careful consideration of middleware order to achieve expected behavior

**Neutral**
- Shifts design toward a more functional programming approach
- Requires developers to understand middleware concepts and composition patterns
