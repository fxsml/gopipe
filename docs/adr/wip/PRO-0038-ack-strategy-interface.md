# ADR 0038: Acknowledgment Strategy Interface

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #2)

## Context

NATS (fire-and-forget), Kafka (offset-based), and RabbitMQ (per-message) have incompatible acknowledgment models. A unified interface is needed to translate gopipe's ack/nack to broker semantics.

## Decision

Introduce `AckStrategy` interface that adapters implement:

```go
// AckStrategy defines how acknowledgments are handled.
type AckStrategy interface {
    Mode() AckMode
    OnAck(ctx context.Context, msg *Message) error
    OnNack(ctx context.Context, msg *Message, err error) error
    OnBatchAck(ctx context.Context, msgs []*Message) ([]*Message, error)
}

type AckMode int

const (
    AckModeNone       AckMode = iota // NATS Core
    AckModePerMessage                // RabbitMQ, NATS JetStream
    AckModeOffset                    // Kafka
    AckModeBatch                     // RabbitMQ multiple flag
)

type AckBehavior struct {
    AutoAck               bool
    AckOnOutput           bool
    RequeueOnNack         bool
    MaxRedeliveries       int
    DeadLetterDestination string
}
```

## Consequences

**Positive:**
- Unified acknowledgment API
- Broker-specific optimizations possible
- Supports batch acking for Kafka

**Negative:**
- Additional abstraction layer
- Adapter complexity increases

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)
- Related: [IMP-0017](IMP-0017-message-acknowledgment.md) - Message Acknowledgment
