# ADR 0014: Composable Pipe Architecture

**Date:** 2024-10-01
**Status:** Implemented

## Context

The library needs clear separation between pipeline configuration and runtime execution. Processing functions mixed configuration with execution logic, making composition difficult.

## Decision

1. Introduce Pipe interface with Start method:
```go
type Pipe[Pre, Out any] interface {
    Start(ctx context.Context, pre <-chan Pre) <-chan Out
}
```

2. Introduce PreProcessorFunc for input transformation:
```go
type PreProcessorFunc[Pre, In any] func(in <-chan Pre) <-chan In
```

3. Change Processor.Process to return slice of outputs (supports filtering, expanding).

## Consequences

**Positive:**
- Separates configuration from execution
- Enables pipeline reuse across contexts
- Supports zero, one, or multiple outputs per input

**Negative:**
- Breaking change for existing Processor implementations
- Performance overhead for single-output operations

## Links

- Related: ADR 0013, ADR 0015
