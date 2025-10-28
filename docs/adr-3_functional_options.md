## ADR-3: Functional Options for Pipe Construction

### Context

Pipe configuration requires flexibility and extensibility. Two main patterns exist: passing a config struct or using functional options. Config structs are explicit but rigid, and require versioning and migration for new fields. Functional options allow incremental, composable configuration and avoid breaking changes.

### Decision

Pipe constructors accept a variadic list of functional options:

```go
func NewPipe[Pre, In, Out any](preProc PreProcessorFunc[Pre, In], proc Processor[In, Out], opts ...Option[In, Out]) Pipe[Pre, Out]
```

Each option is a function that mutates an internal config struct. Options can be composed, reused, and extended without changing the constructor signature. This pattern is used for concurrency, buffering, timeouts, middleware, metrics, and more.


### Consequences

**Positive**
- Composable and extensible configuration
- Avoids breaking changes when adding new features
- Enables defaulting and overrides

**Negative**
- Less discoverable than a config struct
- Order of options may matter for some features

**Neutral**
- Config struct is still used internally, but not exposed

