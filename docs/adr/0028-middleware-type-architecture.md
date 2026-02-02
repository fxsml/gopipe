# ADR 0028: Middleware Type Architecture

**Date:** 2026-02-02
**Status:** Proposed

## Context

The middleware module consolidation (Plan 0016) requires careful type design to avoid requiring explicit type conversions at call sites. Go generics have a limitation: raw function types don't automatically convert to named types in variadic parameters.

The goal is to allow:
```go
router.Use(
    middleware.Retry[*Message, *Message](cfg),  // Generic middleware
    msgmw.CorrelationID(),                      // Message-specific middleware
)
```

Without requiring explicit conversions.

## Decision

### Type Ownership

| Package | Defines | Notes |
|---------|---------|-------|
| `pipe` | `ProcessFunc[In, Out]`, `Middleware[In, Out]` | Canonical types |
| `middleware` | **Nothing** | Uses `pipe.ProcessFunc` directly |
| `middleware/message` | Type alias only | `= pipe.Middleware[*Message, *Message]` |

### Why This Works

1. `pipe.ProcessFunc[In, Out]` is the single source of truth
2. Middleware returns raw function type: `func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]`
3. Raw function types are **assignable** to `pipe.Middleware` without explicit conversion
4. Type aliases (`=`) are fully interchangeable, not new types

### Generic Middleware

```go
package middleware

import "github.com/fxsml/gopipe/pipe"

// NO type definitions - uses pipe.ProcessFunc directly

func Retry[In, Out any](cfg RetryConfig) func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out] {
    return func(next pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out] {
        return func(ctx context.Context, in In) ([]Out, error) {
            // retry logic
        }
    }
}
```

### Message Middleware

```go
package message // middleware/message

import "github.com/fxsml/gopipe/pipe"

// Type ALIAS (not new type) - fully interchangeable
type Middleware = pipe.Middleware[*message.Message, *message.Message]

// Use helper accepts both generic and message middleware
func Use(fn pipe.ProcessFunc[*message.Message, *message.Message], mw ...Middleware) pipe.ProcessFunc[*message.Message, *message.Message]

func CorrelationID() func(pipe.ProcessFunc[*message.Message, *message.Message]) pipe.ProcessFunc[*message.Message, *message.Message]
```

### What Doesn't Work

```go
// RAW function types - FAILS with Go generics
func Retry[In, Out any]() func(func(context.Context, In) ([]Out, error)) func(context.Context, In) ([]Out, error)

pipe.Use(processor, Retry[string, string]())  // ERROR: type mismatch
```

The raw function signature `func(context.Context, In) ([]Out, error)` is not assignable to `pipe.ProcessFunc[In, Out]` in variadic generic parameters.

## Consequences

**Breaking Changes:**
- `middleware` package must import `pipe` (unavoidable)

**Benefits:**
- No type conversions at call sites
- Clean, intuitive API
- Generic and message middleware interoperate seamlessly

**Drawbacks:**
- `middleware` depends on `pipe` (cannot be fully independent)
- Type alias pattern may be unfamiliar to some developers

## Links

- Related: [Plan 0016 - Middleware Module Consolidation](../plans/0016-middleware-consolidation.md)
- Related: ADR 0017 (Middleware for ProcessFunc)
