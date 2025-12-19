# PRO-0013: Middleware Package Consolidation

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0037

## Overview

Consolidate all middleware into a dedicated `middleware/` package with clear organization.

## Goals

1. Create `middleware/` package structure
2. Move existing middleware from root package
3. Add message-specific middleware subpackage
4. Provide backward compatibility

## Task 1: Package Structure

**Goal:** Create middleware package hierarchy

```
middleware/
├── middleware.go       # Core types (Middleware, Chain)
├── timeout.go          # Context timeout middleware
├── retry.go            # Retry middleware (moved from root)
├── metrics.go          # Metrics collection middleware
├── logging.go          # Logging middleware
├── recover.go          # Panic recovery middleware
├── tracing.go          # OpenTelemetry tracing middleware
├── message/            # Message-specific middleware
│   ├── correlation.go  # Correlation ID propagation
│   ├── validation.go   # CloudEvents validation
│   └── routing.go      # Error routing middleware
└── doc.go              # Package documentation
```

**Files to Create:**
- `middleware/middleware.go` - Core types
- `middleware/timeout.go` - Timeout middleware
- `middleware/retry.go` - Moved from root
- `middleware/recover.go` - Recovery middleware
- `middleware/doc.go` - Package docs
- `middleware/message/` - Subpackage

## Task 2: Core Middleware Types

```go
// middleware/middleware.go
package middleware

import "github.com/fxsml/gopipe"

type Middleware[In, Out any] = gopipe.Middleware[In, Out]

func Chain[In, Out any](middleware ...Middleware[In, Out]) Middleware[In, Out] {
    return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
        for i := len(middleware) - 1; i >= 0; i-- {
            next = middleware[i](next)
        }
        return next
    }
}
```

## Task 3: Migration from Root

| Old Location | New Location |
|--------------|--------------|
| `gopipe.WithRetryConfig` | `middleware.WithRetry` |
| `gopipe.RetryConfig` | `middleware.RetryConfig` |
| `gopipe.BackoffFunc` | `middleware.BackoffFunc` |
| `gopipe.useRecover` (internal) | `middleware.WithRecover` |

## Task 4: Backward Compatibility

```go
// gopipe/deprecated.go
package gopipe

import "github.com/fxsml/gopipe/middleware"

// Deprecated: Use middleware.WithRetry instead
func WithRetryConfig[In, Out any](config RetryConfig) Option[In, Out] {
    return WithMiddleware(middleware.WithRetry[In, Out](middleware.RetryConfig(config)))
}
```

**Acceptance Criteria:**
- [ ] `middleware/` package created
- [ ] Core types defined (Middleware, Chain)
- [ ] Timeout middleware implemented
- [ ] Retry middleware moved from root
- [ ] Recovery middleware implemented
- [ ] Message subpackage created
- [ ] Deprecated wrappers in root package
- [ ] Tests pass
- [ ] CHANGELOG updated

## Related

- [PRO-0037](../../adr/PRO-0037-middleware-package-consolidation.md) - ADR
- [PRO-0008](../PRO-0008-separate-middleware-from-config/) - Config/Middleware separation
