# PRO-0022: Extension Points via Middleware

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0043

## Overview

Implement common extension middleware: dead letter, deduplication, tracing.

## Goals

1. Implement `DeadLetterMiddleware`
2. Implement `DeduplicationMiddleware`
3. Implement `TracingMiddleware`
4. Define `Hook` interface

## Task 1: Dead Letter Middleware

**Files to Create:**
- `middleware/deadletter.go`

```go
func DeadLetterMiddleware[In, Out any](
    maxRetries int,
    dlqPublisher func(ctx context.Context, msg *Message, err error) error,
) Middleware[In, Out]
```

## Task 2: Deduplication Middleware

**Files to Create:**
- `middleware/dedup.go`
- `middleware/dedup_store.go`

```go
func DeduplicationMiddleware[In, Out any](
    keyFunc func(*Message) string,
    store DeduplicationStore,
    ttl time.Duration,
) Middleware[In, Out]

type DeduplicationStore interface {
    Seen(ctx context.Context, key string) (bool, error)
    Mark(ctx context.Context, key string, ttl time.Duration) error
}
```

## Task 3: Tracing Middleware

**Files to Create:**
- `middleware/tracing.go`

```go
func TracingMiddleware[In, Out any](
    tracer trace.Tracer,
    spanName string,
) Middleware[In, Out]
```

## Task 4: Hook System

**Files to Create:**
- `hook.go`

```go
type Hook interface {
    BeforeProcess(ctx context.Context, msg *Message) context.Context
    AfterProcess(ctx context.Context, msg *Message, out []*Message, err error)
    OnAck(ctx context.Context, msg *Message)
    OnNack(ctx context.Context, msg *Message, err error)
}
```

**Acceptance Criteria:**
- [ ] Dead letter middleware
- [ ] Deduplication middleware with store interface
- [ ] In-memory dedup store
- [ ] Tracing middleware
- [ ] Hook interface and registry
- [ ] Tests for each
- [ ] CHANGELOG updated

## Related

- [PRO-0043](../../adr/PRO-0043-extension-points-middleware.md) - ADR
