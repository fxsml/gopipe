# PRO-0004: Middleware Package

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0026

## Overview

Consolidate all middleware into a dedicated `middleware/` package with clear API.

## Goals

1. Move all middleware to `middleware/` package
2. Provide `middleware.Chain()` for composing middleware
3. Deprecate old middleware functions in root package

## Task

**Goal:** Move all middleware to `middleware/` package

**Target Structure:**
```
middleware/
├── middleware.go       # Core types, Chain()
├── timeout.go          # Context timeout
├── retry.go            # Retry logic
├── recover.go          # Panic recovery
└── doc.go
```

**Migration Table:**
| Old Location | New Location |
|--------------|--------------|
| `gopipe.WithRetryConfig` | `middleware.WithRetry` |
| `gopipe.RetryConfig` | `middleware.RetryConfig` |
| `gopipe.useRecover` | `middleware.WithRecover` |
| `gopipe.useContext` | `middleware.WithTimeout` |

**Usage:**
```go
import "github.com/fxsml/gopipe/middleware"

pipe := NewProcessPipe(handler, ProcessorConfig{
    Middleware: middleware.Chain(
        middleware.WithTimeout(5*time.Second),
        middleware.WithRetry(retryConfig),
        middleware.WithRecover(),
    ),
})
```

**Files to Create/Modify:**
- `middleware/middleware.go` (new) - Core types, Chain()
- `middleware/timeout.go` (new)
- `middleware/retry.go` (move from root)
- `middleware/recover.go` (new)
- `deprecated.go` (modify) - Add deprecated wrappers

**Acceptance Criteria:**
- [ ] All middleware in `middleware/` package
- [ ] `middleware.Chain()` function works
- [ ] Deprecated wrappers in root package
- [ ] Tests for all middleware
- [ ] CHANGELOG updated

## Related

- [PRO-0001](../PRO-0001-processor-config/) - ProcessorConfig (middleware field)
- [PRO-0026](../../adr/PRO-0026-pipe-processor-simplification.md) - Pipe Simplification ADR
