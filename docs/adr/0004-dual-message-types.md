# ADR 0004: Dual Message Types (TypedMessage[T] and Message)

**Date:** 2025-12-07
**Status:** Accepted

## Context

gopipe serves two distinct use cases:

1. **Messaging/Pub-Sub**: Integration with message brokers (Kafka, RabbitMQ, NATS) where payloads are always `[]byte`
2. **Type-Safe Pipelines**: Data transformation pipelines where compile-time type safety is valuable

The original implementation used a single generic type `Message[T any]` for both use cases. This created friction for pub/sub patterns:

```go
// Verbose: Every function requires [[]byte] type parameter
func publisher(msg *message.Message[[]byte]) error { ... }
func subscriber() <-chan *message.Message[[]byte] { ... }

// Compare to Watermill's simpler approach:
func publisher(msg *message.Message) error { ... }
func subscriber() <-chan *message.Message { ... }
```

## Decision

Introduce a dual-type system using a type alias:

1. **TypedMessage[T any]** - Generic message for type-safe pipelines (renamed from `Message[T]`)
2. **Message** - Type alias for `TypedMessage[[]byte]`, providing simpler API for pub/sub

```go
// Generic typed message (renamed from Message[T])
type TypedMessage[T any] struct {
    Payload    T
    Properties map[string]any
    a          *acking
}

// Non-generic message alias for pub/sub
type Message = TypedMessage[[]byte]
```

## Rationale

**Why a type alias instead of separate types?**

- **Zero overhead**: Alias is compile-time only, no runtime cost
- **Code reuse**: All methods and functions work for both types
- **Simplicity**: No duplication of struct definitions or methods
- **Flexibility**: Users can choose based on their use case

**Benefits:**

1. **Pub/Sub Simplicity**: `Message` provides clean API like Watermill
   ```go
   type Publisher interface {
       Publish(ctx context.Context, msgs <-chan *Message) <-chan struct{}
   }
   ```

2. **Type-Safe Pipelines**: `TypedMessage[T]` preserves compile-time guarantees
   ```go
   pipe := gopipe.NewTransformPipe(
       func(ctx context.Context, msg *message.TypedMessage[Order]) (*message.TypedMessage[OrderConfirmed], error) {
           // Full type safety
       },
   )
   ```

3. **Interoperability**: Both types share the same underlying struct
   ```go
   var msg *TypedMessage[[]byte] = someMessage
   var simpleMsg *Message = msg  // Works seamlessly
   ```

## Consequences

### Breaking Changes

**None** - This is a backward-compatible addition:
- Existing code using `New[T]()` continues to work
- Generic functions accept both `TypedMessage[T]` and `Message`
- Type alias means `Message` and `TypedMessage[[]byte]` are identical

### API Surface

**New Types:**
- `TypedMessage[T any]` - Renamed from `Message[T]`
- `Message` - Alias for `TypedMessage[[]byte]`

**Constructors:**
- `New[T any](payload T, opts ...Option[T]) *TypedMessage[T]` - Works for all types
- `NewTyped[T any](payload T, opts ...Option[T]) *TypedMessage[T]` - Explicit alias

**Pub/Sub Interfaces** now use non-generic `Message`:
```go
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) <-chan struct{}
}

type Subscriber interface {
    Subscribe(ctx context.Context, topic string) <-chan *Message
}

type Router interface {
    Start(ctx context.Context, msgs <-chan *Message) <-chan *Message
}
```

### Migration Guide

**For typed pipelines** - Use `TypedMessage[T]` explicitly:
```go
// Before: Implicit generic inference
msg := message.New(42, message.WithID[int]("x"))

// After: More explicit with TypedMessage
msg := message.New(42, message.WithID[int]("x"))  // Still works!
// Or explicitly:
msg := message.NewTyped(42, message.WithID[int]("x"))

func process(msg *message.TypedMessage[int]) error { ... }
```

**For pub/sub messaging** - Use `Message` (alias):
```go
// Before: Verbose
func publisher(msg *message.Message[[]byte]) error { ... }

// After: Clean
func publisher(msg *message.Message) error { ... }
```

### Documentation Updates

- Updated function signatures in `pubsub.go` and `router.go`
- All pub/sub examples use `Message`
- Type-safe pipeline examples use `TypedMessage[T]`
- README clarifies when to use each type

## Examples

### Pub/Sub Pattern (using Message)

```go
// Publisher
publisher := message.NewPublisher(sender,
    func(msg *message.Message) string {
        subject, _ := message.SubjectProps(msg.Properties)
        return subject
    },
    message.PublisherConfig{},
)

// Subscriber
subscriber := message.NewSubscriber(receiver, message.SubscriberConfig{})
msgs := subscriber.Subscribe(ctx, "orders")

for msg := range msgs {
    var order Order
    json.Unmarshal(msg.Payload, &order)
    // Process...
    msg.Ack()
}
```

### Type-Safe Pipeline (using TypedMessage[T])

```go
pipe := gopipe.NewTransformPipe(
    func(ctx context.Context, msg *message.TypedMessage[Order]) (*message.TypedMessage[OrderConfirmed], error) {
        confirmed := OrderConfirmed{ID: msg.Payload.ID}
        return message.Copy(msg, confirmed), nil
    },
)
```

## Alternatives Considered

### 1. Separate Struct Types
**Rejected**: Would require duplicating all methods and create type conversion overhead

### 2. Single Generic Type Only
**Rejected**: Verbose for pub/sub use cases, doesn't align with standard messaging libraries

### 3. Interface-Based Payload
**Rejected**: Loses compile-time type safety, requires runtime type assertions

## References

- [Watermill vs gopipe Comparison](../watermill-comparison.md)
- [Message Refactoring Proposal](../message-refactoring-proposal.md)
- [ADR 0001: Public Message Fields](./0001-public-message-fields.md)
- [ADR 0002: Remove Properties Thread-Safety](./0002-remove-properties-thread-safety.md)
