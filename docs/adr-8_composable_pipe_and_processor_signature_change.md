## ADR-8: Composable Pipe and Processor Signature Change

See #8

### Context

The gopipe library needs to establish a clear separation between pipeline configuration and runtime execution. Currently, the advanced processing functions (Process, Batch, etc.) mix configuration options with execution logic, making them difficult to compose and extend. Additionally, there's no unified approach for constructing advanced pipeline components with consistent behavior.

Furthermore, the Processor signature returns a single output per input, which limits flexibility for operations that may produce multiple or no outputs.

### Decision

1. Introduce a Pipe interface with a Start method that initiates processing:

```go
type Pipe[Pre, Out any] interface {
    Start(ctx context.Context, pre <-chan Pre) <-chan Out
}
```

2. Introduce a PreProcessorFunc type to transform inputs before they reach the main processor:

```go
type PreProcessorFunc[Pre, In any] func(in <-chan Pre) <-chan In
```

3. Create a NewPipe constructor function for building composable pipelines:

```go
func NewPipe[Pre, In, Out any](
    preProc PreProcessorFunc[Pre, In],
    proc Processor[In, Out],
    opts ...Option[In, Out],
) Pipe[Pre, Out]
```

4. Change the Processor.Process signature to return a slice of outputs instead of a single output:

```go
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)
}
```

### Consequences

**Positive**
- Clearly separates pipeline configuration (pipe construction with options) from runtime execution (Start method)
- Enables reuse of configured pipelines across multiple executions with different contexts
- Unifies the composition model for all advanced functions (Transform, Process, Batch) 
- Provides consistent context propagation and cancellation across the entire pipeline
- Supports producing zero, one, or multiple outputs from a single input (filtering, expanding, transforming)
- PreProcessorFunc allows type conversion and transformations before main processing
- Enables seamless connection between different data types in a pipeline

**Negative**
- Breaking change for code using the previous Processor interface
- Small performance overhead for operations that always produce exactly one output
- Increased complexity in the core abstraction layer

**Neutral**
- Specialized pipe implementations (TransformPipe, ProcessPipe, BatchPipe) help bridge the gap between simple and advanced use cases
- Migration requires updating existing processor implementations to return slices
- NewPipe constructor establishes a standard pattern for building all advanced pipeline components
- The configuration/runtime split allows for more sophisticated testing and reuse scenarios
- PreProcessorFunc adds complexity but enables critical use cases like batch collection (Collect → BatchPipe) and batch flattening (Flatten → ProcessPipe)
