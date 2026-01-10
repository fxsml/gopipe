# ADR 0015: Remove Cancel Path

**Date:** 2025-12-19
**Status:** Accepted

## Context

The current `Processor` interface includes a `Cancel` method that is called in two scenarios:
1. When `Process` returns an error
2. When context is cancelled - a goroutine drains remaining inputs and calls `Cancel` for each

This draining behavior adds complexity and the `Processor` type becomes unnecessary overhead. A simple handler function with `Pipe` provides the same functionality.

## Decision

Remove the draining cancel path. The key changes:

1. **Remove `CancelFunc` and `Processor.Cancel`** - No more separate error handling path
2. **Handler called for ALL inputs** - Every input from the channel will invoke the handler
3. **Context checking is handler's responsibility** - If the handler wants to skip processing when cancelled, it must check `ctx.Done()` itself (not provided out of the box)

```go
// Current: Processor with Cancel method
type Processor[In, Out any] interface {
    Process(context.Context, In) ([]Out, error)
    Cancel(In, error)  // Called on error or context cancellation
}

// After: Just ProcessFunc
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)
```

Usage with existing pipe constructors:

```go
// Transform each input (1:1)
p := pipe.NewTransformPipe(func(ctx context.Context, n int) (string, error) {
    return strconv.Itoa(n * 2), nil
}, pipe.Config{})
out, err := p.Start(ctx, input)

// Process with multiple outputs (1:N)
p := pipe.NewProcessPipe(func(ctx context.Context, s string) ([]int, error) {
    return []int{len(s), len(s) * 2}, nil
}, pipe.Config{})
out, err := p.Start(ctx, input)
```

Handler with context cancellation check (optional, not provided out of the box):

```go
func handler(ctx context.Context, n int) (string, error) {
    // Handler is called for ALL inputs, even after cancellation.
    // Check context yourself if you want to skip processing:
    select {
    case <-ctx.Done():
        return "", ctx.Err()  // Or return zero value and nil to drop silently
    default:
    }

    return strconv.Itoa(n * 2), nil
}
```

## Consequences

**Breaking Changes:**
- Code using `Cancel` callback must be updated

**Benefits:**
- Simpler mental model - no separate cancel path to understand
- Reduces API surface (no `CancelFunc`, no `Processor` interface needed)
- Handler functions are easier to test
- Predictable: handler always called for every input

**Drawbacks:**
- No built-in draining with error routing (dead-letter patterns need custom implementation)
- Handler must explicitly check context if cancellation behavior is needed

## Links

- Supersedes: ADR 0001 (Processor Abstraction)
- Related: ADR 0016, ADR 0017

## Updates

**2025-12-22:** Updated examples to show `Start` returning error, added `pipe.Config{}` parameter. Updated Consequences format to match ADR template.
