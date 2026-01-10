# ADR 0017: Middleware for ProcessFunc

**Date:** 2025-12-19
**Status:** Accepted

## Context

With the removal of the `Processor` type (see ADR 0015), middleware must target the `ProcessFunc` signature directly. This provides a simpler, more composable approach to cross-cutting concerns.

## Decision

Define middleware as functions that wrap `ProcessFunc`:

```go
// ProcessFunc is the core processing signature.
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

// Middleware wraps a ProcessFunc with additional behavior.
type Middleware[In, Out any] func(ProcessFunc[In, Out]) ProcessFunc[In, Out]
```

Pipes store middleware and apply them at `Start` time. `Use` can be called multiple times; middleware executes in the order added.

Example middleware - context:

```go
type ContextConfig struct {
    Timeout     time.Duration
    Propagation bool  // If false, use background context
}

// Context wraps a ProcessFunc with context management.
func Context[In, Out any](cfg ContextConfig) Middleware[In, Out] {
    return func(next ProcessFunc[In, Out]) ProcessFunc[In, Out] {
        return func(ctx context.Context, in In) ([]Out, error) {
            if !cfg.Propagation {
                ctx = context.Background()
            }
            if cfg.Timeout > 0 {
                var cancel context.CancelFunc
                ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
                defer cancel()
            }
            return next(ctx, in)
        }
    }
}
```

Usage:

```go
// Create pipe and apply middleware
p := pipe.NewTransformPipe(myHandler, pipe.Config{})
err := p.Use(middleware.Context[int, string](middleware.ContextConfig{
    Timeout:     5 * time.Second,
    Propagation: true,
}))

// Start returns error if pipe already started
out, err := p.Start(ctx, input)
```

## Consequences

**Benefits:**
- Simple function composition - no interfaces
- Type-safe with generics
- Easy to test middleware in isolation
- Standard Go pattern (similar to http.Handler middleware)
- Middleware can be reused across different handlers

**Drawbacks:**
- Generic type parameters required on each middleware
- No access to processor-level concerns (only per-message)
- Middleware order matters and can be confusing

## Links

- Supersedes: ADR 0002 (Middleware Pattern)
- Related: ADR 0015, ADR 0016

## Updates

**2025-12-22:** Fixed `ProcessFunc` signature to `([]Out, error)`. Updated usage to show `Use` returning error. Added note about middleware storage and execution order. Updated Consequences format to match ADR template.
