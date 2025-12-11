# ADR 0009: Transactional Outbox Pattern

**Date:** 2025-12-08
**Status:** Proposed

## Context

The dual-write problem: saving to database and publishing a message are separate operations. If one fails after the other succeeds, the system becomes inconsistent. The transactional outbox pattern solves this by storing messages in the same database transaction as business data.

## Decision

Implement outbox pattern for exactly-once semantics:

```go
type OutboxStore interface {
    Store(ctx context.Context, tx Transaction, msg *message.Message) error
    GetPending(ctx context.Context, limit int) ([]*OutboxMessage, error)
    MarkPublished(ctx context.Context, id string) error
}

type OutboxProcessor struct {
    outbox    OutboxStore
    publisher MessagePublisher
    interval  time.Duration
}
```

Flow:
1. Business logic and outbox insert in same DB transaction
2. Background processor polls outbox
3. Publishes pending messages to broker
4. Marks messages as published

## Consequences

**Positive:**
- Exactly-once delivery semantics
- Atomic database + messaging
- Survives process restarts

**Negative:**
- Requires database
- Background processor needed
- Additional operational complexity
- Only for critical (<1%) use cases

## Links

- ADR 0006: CQRS Implementation
- ADR 0008: Compensating Saga Pattern
- Future package: `cqrs/outbox`
