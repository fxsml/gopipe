# ADR-24: Message Acknowledgment Pattern

## Status

Accepted

## Context

When integrating gopipe with message brokers (RabbitMQ, Kafka, SQS), we need explicit acknowledgment of processing outcomes to enable at-least-once delivery guarantees and automatic redelivery on failures. The challenge is providing this while maintaining gopipe's clean API and avoiding error-prone manual acknowledgment in user code.

## Decision

Implemented `Message[T]` type with automatic acknowledgment:

```go
type Message[T any] struct {
    Metadata Metadata
    Payload  T
}

func NewMessage[T any](metadata Metadata, payload T, deadline time.Time, 
                       ack func(), nack func(error)) *Message[T]
func (m *Message[T]) Ack() bool
func (m *Message[T]) Nack(err error) bool
func CopyMessage[In, Out any](msg *Message[In], payload Out) *Message[Out]
func NewMessagePipe[In, Out any](handle func(context.Context, In) ([]Out, error), 
                                  opts ...Option) Pipe
```

**Key behaviors:**
- Thread-safe, mutually exclusive ack/nack operations
- Idempotent - callbacks called at most once
- Automatic via `NewMessagePipe`: success → Ack(), failure → Nack(err)
- Output messages share acknowledgment state via `CopyMessage`

**User code remains clean:**
```go
pipe := gopipe.NewMessagePipe(func(ctx context.Context, value int) ([]int, error) {
    return []int{value * 2}, nil  // No manual ack/nack needed
})
```

## Consequences

**Positive:**
- Clean user code - framework handles ack/nack automatically
- Thread-safe with mutex protection, passes race detector
- Works with all existing pipe options
- Optional callbacks (can be nil)

**Trade-offs:**
- Requires wrapping payloads in Message type
- Output messages share input's acknowledgment state (suitable for 1-to-many)
- Users must write idempotent handlers for at-least-once semantics
