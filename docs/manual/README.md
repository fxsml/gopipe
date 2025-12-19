# gopipe Manual

A composable channel processing and CloudEvents messaging framework for Go.

## Quick Links

- [Getting Started](#getting-started)
- [Current State](#current-state)
- [Upcoming Changes](#upcoming-changes)
- [Architecture Roadmap](../plans/architecture-roadmap.md)

---

## Getting Started

```go
import "github.com/fxsml/gopipe"
```

### Basic Pipeline

```go
// Transform each input
pipe := gopipe.NewTransformPipe(func(ctx context.Context, n int) (string, error) {
    return fmt.Sprintf("value: %d", n), nil
})

in := make(chan int)
out := pipe.Start(ctx, in)
```

### Processing Pipeline

```go
// Zero, one, or many outputs per input
pipe := gopipe.NewProcessPipe(func(ctx context.Context, n int) ([]string, error) {
    if n < 0 {
        return nil, nil  // Filter out
    }
    return []string{fmt.Sprint(n)}, nil
})
```

### Messaging

```go
// Router-based message handling
router := message.NewRouter(message.RouterConfig{})
router.AddHandler(myHandler)

msgs := subscriber.Subscribe(ctx)
processed := router.Start(ctx, msgs)
```

---

## Current State

### Message Types

```go
// TypedMessage[T] - generic wrapper for any data type
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
}

// Message - alias for pub/sub (DEPRECATED alias, see Upcoming Changes)
type Message = TypedMessage[[]byte]

// Constructor (DEPRECATED signature, see Upcoming Changes)
msg := message.New(data, attrs)
```

### Pipe Options

```go
// Current: Generic functional options (DEPRECATED, see Upcoming Changes)
pipe := gopipe.NewProcessPipe(
    handler,
    gopipe.WithConcurrency[In, Out](4),
    gopipe.WithTimeout[In, Out](5*time.Second),
)
```

### Channel Operations

```go
import "github.com/fxsml/gopipe/channel"

// Merge multiple channels
merged := channel.Merge(ch1, ch2, ch3)

// Broadcast to multiple consumers
outputs := channel.Broadcast(input, 3)

// Collect into batches
batches := channel.Collect(input, 100, time.Second)
```

---

## Upcoming Changes

> **Reference:** [Architecture Roadmap](../plans/architecture-roadmap.md)

### Breaking Changes Summary

| Component | Current | Future | Plan |
|-----------|---------|--------|------|
| `Message` | `TypedMessage[[]byte]` | `TypedMessage[any]` | [Layer 1](../plans/layer-1-message-standardization.md) |
| `New[T]()` | Returns `*TypedMessage[T]` | **Deprecated** → `NewTyped[T]()` | [Layer 1](../plans/layer-1-message-standardization.md) |
| `New()` | N/A | Returns `(*Message, error)` | [Layer 1](../plans/layer-1-message-standardization.md) |
| `Option[In,Out]` | Generic options | `ProcessorConfig` struct | [Layer 0](../plans/layer-0-foundation-cleanup.md) |
| `Generator[T]` | Generator interface | `Subscriber[T]` | [Layer 0](../plans/layer-0-foundation-cleanup.md) |

### TypedMessage Decision

`TypedMessage[T]` is **preserved** for non-messaging pipelines:

```go
// Future: Use NewTyped for pipelines (no validation)
msg := message.NewTyped(order, nil)

// Future: Use New for CloudEvents (validates, returns error)
msg, err := message.New(order, message.Attributes{
    message.AttrType:   "order.created",
    message.AttrSource: "/orders",
})
```

### CloudEvents Mandatory

Future messages require CloudEvents attributes:
- `id` - Unique identifier
- `source` - URI reference
- `specversion` - "1.0"
- `type` - Event type

### ProcessorConfig

```go
// Future: Struct config instead of generic options
pipe := gopipe.NewProcessPipe(handler, gopipe.ProcessorConfig{
    Concurrency: 4,
    Timeout:     5 * time.Second,
})
```

---

## Package Structure

```
gopipe/
├── pipe.go              # Pipe interface and constructors
├── processor.go         # Processor abstraction
├── channel/             # Channel operations (merge, broadcast, etc.)
├── message/             # Message types and pub/sub
│   ├── message.go       # TypedMessage, Message
│   ├── router.go        # Message router
│   ├── broker/          # HTTP, IO brokers
│   └── cqrs/            # Command/Query handlers
└── middleware/          # Middleware components
```

---

## Further Reading

- [ADR Index](../adr/README.md) - Architecture decisions
- [Implementation Plans](../plans/README.md) - Roadmap and sub-plans
- [CHANGELOG](../../CHANGELOG.md) - Implemented features by version
- [Patterns](../patterns/) - CQRS, Saga, Outbox patterns
