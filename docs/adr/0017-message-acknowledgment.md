# ADR 0017: Message Acknowledgment Pattern

**Date:** 2024-10-01
**Status:** Implemented

## Context

Integrating with message brokers (RabbitMQ, Kafka, SQS) requires explicit acknowledgment of processing outcomes for at-least-once delivery guarantees. Supporting heterogeneous payloads requires a non-generic approach.

## Decision

Implement non-generic `Message` type with automatic acknowledgment in `message` subpackage:

```go
type Message struct {
    Payload any
}

func (m *Message) Ack() bool
func (m *Message) Nack(err error) bool
```

Key behaviors:
- Thread-safe, mutually exclusive ack/nack
- Idempotent - callbacks called at most once
- Automatic via `NewMessagePipe`: success -> Ack(), failure -> Nack(err)
- Output messages share acknowledgment state via `Copy`

## Consequences

**Positive:**
- Framework handles ack/nack automatically
- Thread-safe with mutex protection
- Works with all existing gopipe options

**Negative:**
- No compile-time type safety (runtime type assertions)
- Users must write idempotent handlers

## Links

- Package: `github.com/fxsml/gopipe/message`
