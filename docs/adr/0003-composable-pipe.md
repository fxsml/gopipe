# ADR 0003: Composable Pipe Architecture

**Date:** 2025-10-01
**Status:** Implemented

## Context

The library needs clear separation between pipeline configuration and runtime execution. Processing functions mixed configuration with execution logic, making composition difficult.

## Decision

1. Introduce Pipe interface with Start method:
```go
type Pipe[Pre, Out any] interface {
    Start(ctx context.Context, pre <-chan Pre) (<-chan Out, error)
}
```

Start returns error (e.g., `ErrAlreadyStarted`) to prevent reuse of stateful pipes.

2. Introduce PreProcessorFunc for input transformation:
```go
type PreProcessorFunc[Pre, In any] func(in <-chan Pre) <-chan In
```

3. Change Processor.Process to return slice of outputs (supports filtering, expanding).

## Consequences

**Breaking Changes:**
- Existing Processor implementations must be updated

**Benefits:**
- Separates configuration from execution
- Enables pipeline reuse across contexts
- Supports zero, one, or multiple outputs per input

**Drawbacks:**
- Performance overhead for single-output operations

## Links

- Related: ADR 0015, ADR 0016, ADR 0017

## Updates

**2025-12-22:** Updated `Start` signature to return error. Updated Consequences format to match ADR template.
