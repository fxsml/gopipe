# PRO-0011: Message-Specific Error Routing

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0035

## Overview

Implement error routing middleware for messaging systems to route errors to appropriate destinations (DLQ, error topics).

## Goals

1. Create error routing middleware for messages
2. Support configurable destinations
3. Include context (original input, stack trace)

## Task

**Goal:** Implement `WithErrorRouting` middleware

**Target:**
```go
// middleware/message/routing.go
package message

type ErrorRoutingConfig struct {
    Destination    string // e.g., "gopipe://errors", "kafka://dlq"
    IncludeInput   bool   // Include original message in error
    IncludeStack   bool   // Include stack trace
}

func WithErrorRouting(config ErrorRoutingConfig) middleware.Middleware[*message.Message, *message.Message]
```

**Usage:**
```go
loop.Route("orders", handler,
    ProcessorConfig{Concurrency: 4},
    WithErrorRouting(ErrorRoutingConfig{
        Destination:  "gopipe://errors",
        IncludeInput: true,
        IncludeStack: true,
    }),
)
```

**Files to Create/Modify:**
- `middleware/message/routing.go` (new) - Error routing middleware
- `middleware/message/doc.go` (new) - Package documentation

**Acceptance Criteria:**
- [ ] `ErrorRoutingConfig` struct defined
- [ ] `WithErrorRouting` middleware implemented
- [ ] Catches errors from wrapped processor
- [ ] Creates error message with context
- [ ] Routes to configured destination
- [ ] Tests for error routing
- [ ] CHANGELOG updated

## Related

- [PRO-0035](../../adr/PRO-0035-message-error-routing.md) - ADR
- [PRO-0013](../PRO-0013-middleware-package-consolidation/) - Middleware package
