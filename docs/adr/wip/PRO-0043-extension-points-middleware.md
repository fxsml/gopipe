# ADR 0043: Extension Points via Middleware

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #7)

## Context

gopipe doesn't provide DLQ, tracing, deduplication, etc. Need extension points via middleware and hooks.

## Decision

Define extension middleware and hook system:

```go
// Dead letter queue
func DeadLetterMiddleware[In, Out any](
    maxRetries int,
    dlqPublisher func(ctx context.Context, msg *Message, err error) error,
) Middleware[In, Out]

// Deduplication
func DeduplicationMiddleware[In, Out any](
    keyFunc func(*Message) string,
    store DeduplicationStore,
    ttl time.Duration,
) Middleware[In, Out]

type DeduplicationStore interface {
    Seen(ctx context.Context, key string) (bool, error)
    Mark(ctx context.Context, key string, ttl time.Duration) error
}

func NewInMemoryDeduplicationStore() DeduplicationStore
func NewRedisDeduplicationStore(client *redis.Client) DeduplicationStore

// Tracing
func TracingMiddleware[In, Out any](
    tracer trace.Tracer,
    spanName string,
) Middleware[In, Out]
```

**Hook System:**

```go
type Hook interface {
    BeforeProcess(ctx context.Context, msg *Message) context.Context
    AfterProcess(ctx context.Context, msg *Message, out []*Message, err error)
    OnAck(ctx context.Context, msg *Message)
    OnNack(ctx context.Context, msg *Message, err error)
}

type HookRegistry struct {
    hooks []Hook
}

func (r *HookRegistry) Add(h Hook)
func (r *HookRegistry) BeforeProcess(ctx context.Context, msg *Message) context.Context
func (r *HookRegistry) AfterProcess(ctx context.Context, msg *Message, out []*Message, err error)
```

## Consequences

**Positive:**
- Extensible architecture
- Common patterns provided
- Hook system for custom needs

**Negative:**
- More concepts to learn
- Performance overhead

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)
- Related: [PRO-0037](PRO-0037-middleware-package-consolidation.md)
