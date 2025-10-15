## ADR-12: Processor Abstraction

See #12

### Context

The gopipe library needs a consistent way to handle processing with support for cancellation, ensuring clean control flow and resource management. Currently, processing logic and error handling are separated, making composition and middleware implementation challenging.

### Decision

Introduce the following Processor interface and use it for the composition of advanced processing functionality.

```go
type Processor[In, Out any] interface {
	Process(context.Context, In) (Out, error)
	Cancel(In, error)
}
```

### Consequences

**Positive**
- Creates a unified abstraction for processing and error handling as a single unit
- Enables middleware pattern implementation for cross-cutting concerns
- Improves composability through a standardized interface
- Enhances testability with clear boundaries and mock opportunities

**Negative**
- Adds a small abstraction layer overhead
- Requires refactoring existing code using separate process/cancel functions

**Neutral**
- Migration from function-based to interface-based approach requires updating client code