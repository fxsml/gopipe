# Plan 0016: Middleware Module Consolidation

**Status:** Proposed
**Related:** [0014 - Pipe Interface Refactoring](./0014-pipe-interface-refactoring.md), [0015 - Message Module Refactoring](./0015-message-module-refactoring.md)

## Overview

Consolidate `pipe/middleware` and `message/middleware` into a single top-level `middleware` module with subpackages for domain-specific middleware.

## Motivation

### Current Problems

1. **Discovery confusion** - Developers working on `message/http` don't naturally find `pipe/middleware.Retry`
2. **Third-party coupling** - Metrics middleware wants Prometheus client, but this pollutes core `pipe` package
3. **Unclear boundaries** - Both packages have `recover.go`, unclear which to use
4. **Layering concerns** - `message/http/publisher.go` importing `pipe/middleware` feels like a violation

### Benefits of Consolidation

1. **Single discovery point** - All middleware in one place
2. **Third-party isolation** - Prometheus, OpenTelemetry stay in `middleware`, not core packages
3. **Neutral ground** - No "pipe vs message" import confusion
4. **Clear subpackage separation** - Generic vs message-aware middleware

## Current State

### pipe/middleware

| File | Function | Purpose |
|------|----------|---------|
| `retry.go` | `Retry[In, Out]` | Retry on error with backoff |
| `recover.go` | `Recover[In, Out]` | Recover from panics |
| `context.go` | `WithContext[In, Out]` | Context propagation |
| `log.go` | `Log[In, Out]` | Logging wrapper |
| `metrics.go` | `Metrics[In, Out]` | Latency/count metrics |
| `metadata.go` | `WithMetadata[In, Out]` | Metadata propagation |
| `middleware.go` | Shared types | Common interfaces |

### message/middleware

| File | Function | Purpose |
|------|----------|---------|
| `recover.go` | `Recover` | Recover from panics (message-aware) |
| `correlation.go` | `CorrelationID` | Set/propagate correlation ID |
| `deadline.go` | `Deadline` | Message deadline handling |
| `subject.go` | `Subject` | Subject/topic manipulation |
| `autoack.go` | `AutoAck` | Automatic acknowledgment |
| `validate.go` | `Validate` | Message validation |

## Proposed Structure

```
middleware/
  doc.go              # Package documentation

  # Generic middleware - works with any pipe.ProcessFunc[In, Out]
  # NO type definitions - uses pipe.ProcessFunc directly
  retry.go            # Retry[In, Out]
  recover.go          # Recover[In, Out]
  context.go          # WithContext[In, Out]
  log.go              # Log[In, Out]
  metadata.go         # WithMetadata[In, Out]

  # Third-party integrations (isolated dependencies)
  metrics.go          # Metrics[In, Out] - may import prometheus
  trace.go            # Trace[In, Out] - may import opentelemetry (future)

  message/
    doc.go            # Subpackage documentation
    middleware.go     # Type alias: Middleware = pipe.Middleware[*Message, *Message]
                      # + Use() helper function
    recover.go        # Recover (message-aware panic recovery)
    correlation.go    # CorrelationID
    deadline.go       # Deadline
    subject.go        # Subject
    autoack.go        # AutoAck
    validate.go       # Validate
```

## API Design

### Type Architecture

**Critical constraint:** Go generics require using named types for compatibility. Raw function types don't work with variadic generic parameters.

| Package | Defines | Returns |
|---------|---------|---------|
| `pipe` | `ProcessFunc[In, Out]`, `Middleware[In, Out]` | Canonical types |
| `middleware` | **Nothing** | `func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]` |
| `middleware/message` | **Type alias only** | Compatible with `pipe.Middleware[*Message, *Message]` |

**Why this works:**
1. `pipe.ProcessFunc[In, Out]` is the single source of truth
2. Middleware returns `func(pipe.ProcessFunc) pipe.ProcessFunc` (raw function type)
3. This is **assignable** to `pipe.Middleware` without explicit conversion
4. Type aliases (`=`) are fully interchangeable, not new types

### Generic Middleware

```go
package middleware

import "github.com/fxsml/gopipe/pipe"

// NO type definitions here! Uses pipe.ProcessFunc directly.

// Retry wraps a ProcessFunc with retry logic
// Returns raw function type - compatible with pipe.Middleware
func Retry[In, Out any](cfg RetryConfig) func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out] {
    return func(next pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out] {
        return func(ctx context.Context, in In) ([]Out, error) {
            // retry logic
        }
    }
}

// Recover wraps a ProcessFunc with panic recovery
func Recover[In, Out any]() func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]

// Log wraps a ProcessFunc with logging
func Log[In, Out any](logger Logger) func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]

// Metrics wraps a ProcessFunc with metrics collection
func Metrics[In, Out any](opts ...MetricsOption) func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]
```

### Message-Aware Middleware

```go
package message // middleware/message

import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pipe"
)

// Middleware is a type ALIAS (not new type) for convenience
// Fully interchangeable with pipe.Middleware[*message.Message, *message.Message]
type Middleware = pipe.Middleware[*message.Message, *message.Message]

// Use applies middleware to a message processor
// Accepts both generic and message-specific middleware without conversion
func Use(fn pipe.ProcessFunc[*message.Message, *message.Message], mw ...Middleware) pipe.ProcessFunc[*message.Message, *message.Message]

// CorrelationID ensures messages have correlation IDs
func CorrelationID() func(pipe.ProcessFunc[*message.Message, *message.Message]) pipe.ProcessFunc[*message.Message, *message.Message]

// AutoAck automatically acknowledges processed messages
func AutoAck() func(pipe.ProcessFunc[*message.Message, *message.Message]) pipe.ProcessFunc[*message.Message, *message.Message]

// Validate validates messages against a schema
func Validate(validator Validator) func(pipe.ProcessFunc[*message.Message, *message.Message]) pipe.ProcessFunc[*message.Message, *message.Message]
```

### Usage Examples

```go
import (
    "github.com/fxsml/gopipe/middleware"
    msgmw "github.com/fxsml/gopipe/middleware/message"
    "github.com/fxsml/gopipe/message"
)

// Example: Router with mixed middleware
// Router.Use() accepts pipe.Middleware[*Message, *Message]
router := message.NewRouter()
router.Use(
    middleware.Retry[*message.Message, *message.Message](cfg),  // Generic - NO CONVERSION
    middleware.Recover[*message.Message, *message.Message](),   // Generic - NO CONVERSION
    msgmw.CorrelationID(),                                      // Message-specific - NO CONVERSION
    msgmw.AutoAck(),                                            // Message-specific - NO CONVERSION
)

// Example: Using middleware/message.Use helper
processor := msgmw.Use(myProcessFunc,
    middleware.Retry[*message.Message, *message.Message](cfg),
    msgmw.CorrelationID(),
)
```

## Naming Convention

| Type | Naming | Example |
|------|--------|---------|
| Generic middleware | No prefix | `middleware.Retry()` |
| Message middleware | Subpackage import | `msgmw.CorrelationID()` |

No prefixes like `MessageCorrelationID` - subpackage provides context.

## Dependencies

### middleware (top-level)

```go
import (
    "github.com/fxsml/gopipe/pipe"

    // Third-party (optional, for specific middleware)
    "github.com/prometheus/client_golang/prometheus"  // metrics.go only
)
```

### middleware/message

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pipe"
)
```

### Dependency Direction

```
middleware ──────► pipe
    │
    └─► message/
           │
           └────► message
                     │
                     └────► pipe
```

Core packages (`pipe`, `message`, `channel`) never import `middleware`.

## Migration Path

### Phase 1: Create New Structure

1. Create `middleware/` module at repository root
2. Copy generic middleware from `pipe/middleware/`
3. Copy message middleware to `middleware/message/`
4. Add deprecation notices to old packages

### Phase 2: Update Consumers

1. Update `message/http` to use new `middleware` package
2. Update examples
3. Update documentation

### Phase 3: Remove Old Packages

1. Remove `pipe/middleware/` (after deprecation period)
2. Remove `message/middleware/` (after deprecation period)

## Tasks

### Task 1: Create middleware module structure

- Create `middleware/doc.go`
- Create `middleware/message/doc.go`
- Create `middleware/message/middleware.go` with type alias and `Use()` helper

### Task 2: Migrate generic middleware

Move from `pipe/middleware/`:
- `retry.go` → `middleware/retry.go`
- `recover.go` → `middleware/recover.go`
- `context.go` → `middleware/context.go`
- `log.go` → `middleware/log.go`
- `metadata.go` → `middleware/metadata.go`
- `metrics.go` → `middleware/metrics.go`
- All corresponding test files

### Task 3: Migrate message middleware

Move from `message/middleware/`:
- `recover.go` → `middleware/message/recover.go`
- `correlation.go` → `middleware/message/correlation.go`
- `deadline.go` → `middleware/message/deadline.go`
- `subject.go` → `middleware/message/subject.go`
- `autoack.go` → `middleware/message/autoack.go`
- `validate.go` → `middleware/message/validate.go`
- All corresponding test files

### Task 4: Update imports in message/http

Update `message/http/publisher.go` and other consumers.

### Task 5: Add deprecation notices

Add deprecation comments to old packages pointing to new location.

### Task 6: Update documentation

- Update README.md
- Update AGENTS.md
- Update examples

### Task 7: Remove deprecated packages

After deprecation period, remove:
- `pipe/middleware/`
- `message/middleware/`

## Acceptance Criteria

- [ ] `middleware/` module created with generic middleware
- [ ] `middleware/message/` subpackage created with message middleware
- [ ] Both `recover.go` implementations migrated (generic vs message-aware)
- [ ] `message/http` updated to use new package
- [ ] Old packages marked deprecated
- [ ] All tests pass
- [ ] Documentation updated
- [ ] Examples updated

## Considerations

### Type Architecture Decision

**Why `middleware` defines no types:**

Go generics don't automatically convert raw function types to named types in variadic parameters. For example, this fails:

```go
// This does NOT work
func Retry[In, Out any]() func(func(context.Context, In) ([]Out, error)) func(context.Context, In) ([]Out, error)

pipe.Use(processor, Retry[string, string]())  // ERROR: type mismatch
```

However, using `pipe.ProcessFunc` directly works:

```go
// This WORKS
func Retry[In, Out any]() func(pipe.ProcessFunc[In, Out]) pipe.ProcessFunc[In, Out]

pipe.Use(processor, Retry[string, string]())  // OK: assignable to pipe.Middleware
```

**Rule:** `middleware` package MUST import `pipe` and use `pipe.ProcessFunc`. This is unavoidable.

### Dual Recover Middleware

Both packages have `Recover`. After consolidation:

- `middleware.Recover[In, Out]` - generic panic recovery, wraps any pipe
- `middleware/message.Recover` - message-aware, may handle ack/nack on panic

These serve different purposes and should both exist.

### Future: Channel Middleware?

Currently no use case identified. Channel operations are pure functions without error returns. If needed, could add `middleware/channel/` subpackage.

### Third-Party Integration Strategy

Third-party dependencies should be optional:

```go
// metrics.go
// +build prometheus

import "github.com/prometheus/client_golang/prometheus"
```

Or use interfaces that users implement with their preferred library.
