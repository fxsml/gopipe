# ADR-24: Message Acknowledgment Pattern (Non-Generic)

## Status

Accepted

## Context

When integrating gopipe with message brokers (RabbitMQ, Kafka, SQS), we need explicit acknowledgment of processing outcomes to enable at-least-once delivery guarantees and automatic redelivery on failures. The challenge is providing this while maintaining gopipe's clean API and avoiding error-prone manual acknowledgment in user code.

Additionally, supporting heterogeneous message payloads from message brokers requires a non-generic approach where payload types are handled at runtime rather than compile-time.

## Decision

Implemented `Message` type (non-generic) with automatic acknowledgment in a dedicated `message` subpackage:

```go
package message

type Message struct {
    Payload any // Heterogeneous payload support
}

func NewMessage(properties *MessageProperties, payload any, deadline time.Time,
                ack func(), nack func(error)) *Message
func (m *Message) Ack() bool
func (m *Message) Nack(err error) bool
func Copy(msg *Message, payload any) *Message
func NewMessagePipe(handle func(context.Context, any) ([]any, error),
                    opts ...gopipe.Option[*Message, *Message]) gopipe.Pipe[*Message, *Message]
```

**Key behaviors:**
- Thread-safe, mutually exclusive ack/nack operations
- Idempotent - callbacks called at most once
- Automatic via `NewMessagePipe`: success → Ack(), failure → Nack(err)
- Output messages share acknowledgment state via `Copy`
- **Non-generic**: Payload is `any`, requiring runtime type assertions
- **Subpackage**: Separated from core gopipe for clear module boundaries

**User code example:**
```go
pipe := message.NewMessagePipe(func(ctx context.Context, payload any) ([]any, error) {
    // Runtime type assertion
    value, ok := payload.(int)
    if !ok {
        return nil, fmt.Errorf("expected int, got %T", payload)
    }
    return []any{value * 2}, nil  // No manual ack/nack needed
})
```

## Consequences

**Positive:**
- Clean user code - framework handles ack/nack automatically
- Thread-safe with mutex protection, passes race detector
- Works with all existing gopipe options
- Optional callbacks (can be nil)
- Simpler API without generics for message broker integration
- Clear module separation - `message` package for broker integration

**Trade-offs:**
- Requires wrapping payloads in Message type
- Output messages share input's acknowledgment state (suitable for 1-to-many)
- **No compile-time type safety** - requires runtime type assertions in handlers
- **Runtime overhead** - type assertions add small performance cost
- Users must write idempotent handlers for at-least-once semantics

**Migration from generic design:**
- Previous generic `Message[T]` removed
- Simpler for message broker use cases with heterogeneous payloads
- Type safety traded for flexibility
