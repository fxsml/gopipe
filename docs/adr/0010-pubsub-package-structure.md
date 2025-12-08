# ADR 0010: Pub/Sub Package Structure

**Date:** 2025-12-08
**Status:** Accepted
**Implementation Date:** 2025-12-08
**Related:**
- [ADR-25: Interface Broker](../adr-25_broker.md)
- [Critical Analysis: Message Design](../critical-analysis-message-design.md)

## Context

The current broker implementation lives in `message/broker/` with interfaces defined in `message/pubsub.go`. This creates several issues:

### Current Structure Problems

1. **Naming confusion**: `broker.NewBroker()` is ambiguous
   ```go
   import "github.com/fxsml/gopipe/message/broker"
   b := broker.NewBroker(config)  // Which broker? In-memory? HTTP?
   ```

2. **Package organization**: Pub/sub interfaces mixed with core message types
   ```go
   message/
   ├── message.go     ← Core: TypedMessage[T], Message
   ├── properties.go  ← Core: Properties helpers
   ├── router.go      ← Core: Router, Handler
   └── pubsub.go      ← Pub/sub: Sender, Receiver, Broker  // ❓ Doesn't belong here
   ```

3. **Import path confusion**: Need both `message` and `message/broker`
   ```go
   import (
       "github.com/fxsml/gopipe/message"
       "github.com/fxsml/gopipe/message/broker"
   )
   ```

4. **Unclear API surface**: What's core messaging vs pub/sub?

### Core vs Pub/Sub Concepts

**Core message concepts** (belong in `message/`):
- `Message` / `TypedMessage[T]` - The message type
- `Properties` - Message metadata
- `Router` / `Handler` - Message routing and processing
- Acking semantics

**Pub/sub concepts** (belong elsewhere):
- `Sender` / `Receiver` - Send/receive abstraction
- `Broker` - Combined sender/receiver
- `Publisher` / `Subscriber` - Channel-based wrappers
- Specific implementations (in-memory, HTTP, IO)

## Decision

Create a dedicated **`pubsub` package** for pub/sub abstractions and implementations.

### New Package Structure

```
pubsub/
├── pubsub.go              ← Interfaces: Sender, Receiver, Broker, Publisher, Subscriber
├── memory/
│   ├── broker.go          ← NewBroker() - in-memory implementation
│   └── broker_test.go
├── http/
│   ├── sender.go          ← NewSender() - HTTP POST sender
│   ├── receiver.go        ← NewReceiver() - HTTP POST receiver
│   ├── http_test.go
│   └── README.md
├── io/
│   ├── sender.go          ← NewSender() - Stream writer
│   ├── receiver.go        ← NewReceiver() - Stream reader
│   ├── broker.go          ← NewBroker() - Combined IO broker
│   ├── io_test.go
│   └── README.md
└── channel/               ← NEW: Zero-copy channel-based broker
    ├── broker.go          ← NewBroker() - Pure Go channels
    ├── broker_test.go
    └── README.md

message/
├── message.go             ← Unchanged: TypedMessage[T], Message
├── properties.go          ← Unchanged: Properties helpers
├── router.go              ← Unchanged: Router, Handler
├── pubsub.go              ← DEPRECATED: Aliases to pubsub package
└── broker/                ← DEPRECATED: Moved to pubsub/memory/, pubsub/http/, etc.
```

### Core Interfaces (pubsub/pubsub.go)

```go
package pubsub

import (
    "context"
    "github.com/fxsml/gopipe/message"
)

// Sender sends messages to topics.
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver receives messages from topics.
type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Broker combines sending and receiving capabilities.
type Broker interface {
    Sender
    Receiver
}

// Publisher provides channel-based message publishing with batching and grouping.
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

// Subscriber provides channel-based message subscription with polling.
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) <-chan *message.Message
}
```

### Implementation Examples

#### In-Memory Broker (pubsub/memory/)

```go
package memory

import "github.com/fxsml/gopipe/pubsub"

// Config configures the in-memory broker.
type Config struct {
    CloseTimeout  time.Duration
    SendTimeout   time.Duration
    ReceiveTimeout time.Duration
    BufferSize    int
}

// NewBroker creates an in-memory broker.
func NewBroker(config Config) pubsub.Broker {
    // Implementation...
}
```

**Usage:**
```go
import "github.com/fxsml/gopipe/pubsub/memory"

broker := memory.NewBroker(memory.Config{
    BufferSize: 100,
})
broker.Send(ctx, "orders/created", msgs)
```

#### HTTP Sender/Receiver (pubsub/http/)

```go
package http

import "github.com/fxsml/gopipe/pubsub"

// NewSender creates an HTTP POST sender.
func NewSender(url string, config Config) pubsub.Sender {
    // Implementation...
}

// NewReceiver creates an HTTP POST receiver.
func NewReceiver(config Config) pubsub.Receiver {
    // Requires HTTP server setup
    // Implementation...
}
```

**Usage:**
```go
import "github.com/fxsml/gopipe/pubsub/http"

sender := http.NewSender("https://api.example.com/messages", http.Config{
    Timeout: 5 * time.Second,
})
sender.Send(ctx, "orders/created", msgs)
```

#### Channel Broker (pubsub/channel/) - NEW!

```go
package channel

import "github.com/fxsml/gopipe/pubsub"

// NewBroker creates a channel-based broker for in-process pub/sub.
// Uses pure Go channels - zero-copy, leverages gopipe channel utilities.
func NewBroker() pubsub.Broker {
    return &channelBroker{
        topics: make(map[string]chan *message.Message),
    }
}
```

**Usage:**
```go
import "github.com/fxsml/gopipe/pubsub/channel"

broker := channel.NewBroker()

// Zero-copy, native Go channels
broker.Send(ctx, "orders/created", msgs)
received, _ := broker.Receive(ctx, "orders/created")
```

**Benefits of channel broker:**
- ✅ Zero-copy (messages stay in memory)
- ✅ Native concurrency (Go channels handle backpressure)
- ✅ Simplest possible (no marshalling, no network)
- ✅ Integrates with gopipe utilities (`channel.FanOut`, `channel.Merge`)
- ✅ Different from memory broker (streaming vs buffering)

## Benefits

### 1. Clear API Surface

```go
// Core messaging (message/)
import "github.com/fxsml/gopipe/message"
msg := message.New(payload, props)
handler := message.NewHandler(...)
router := message.NewRouter(...)

// Pub/sub (pubsub/)
import "github.com/fxsml/gopipe/pubsub/memory"
broker := memory.NewBroker(config)
```

### 2. Explicit Imports

```go
// Before (confusing)
import "github.com/fxsml/gopipe/message/broker"
b := broker.NewBroker(...)  // Which broker?

// After (clear)
import "github.com/fxsml/gopipe/pubsub/memory"
b := memory.NewBroker(...)  // In-memory broker, explicit!
```

### 3. Implementation Isolation

Each implementation in its own subpackage:
- `pubsub/memory` - In-memory broker
- `pubsub/http` - HTTP sender/receiver
- `pubsub/io` - Stream-based sender/receiver
- `pubsub/channel` - Channel-based broker

### 4. Easy to Extend

Adding new implementations:
```
pubsub/
├── kafka/         ← Kafka sender/receiver
├── nats/          ← NATS sender/receiver
├── rabbitmq/      ← RabbitMQ sender/receiver
└── redis/         ← Redis pub/sub
```

### 5. Consistent Naming

```go
memory.NewBroker()          // In-memory broker
http.NewSender()            // HTTP sender
http.NewReceiver()          // HTTP receiver
io.NewBroker()              // IO broker (combined sender/receiver)
channel.NewBroker()         // Channel broker
```

## Comparison with Current Structure

### Before

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/message/broker"
)

// Confusing: NewBroker is generic name
b := broker.NewBroker(config)

// Interfaces mixed with core message
var sender message.Sender = b
```

### After

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pubsub"
    "github.com/fxsml/gopipe/pubsub/memory"
)

// Clear: memory.NewBroker is explicit
b := memory.NewBroker(config)

// Dedicated pub/sub package
var sender pubsub.Sender = b
```

## Migration Path

### Phase 1: Create `pubsub` Package (Non-Breaking)

1. Create `pubsub/pubsub.go` with interfaces
2. Create `pubsub/memory/`, `pubsub/http/`, `pubsub/io/`
3. Move code from `message/broker/*` to `pubsub/*/`
4. Keep `message/pubsub.go` with type aliases:
   ```go
   // Deprecated: Use github.com/fxsml/gopipe/pubsub instead
   type Sender = pubsub.Sender
   type Receiver = pubsub.Receiver
   type Broker = pubsub.Broker
   ```
5. Keep `message/broker/*` with forwarding functions:
   ```go
   // Deprecated: Use github.com/fxsml/gopipe/pubsub/memory instead
   func NewBroker(config Config) message.Broker {
       return memory.NewBroker(memory.Config{...})
   }
   ```

### Phase 2: Update Examples and Documentation

1. Update all examples to use `pubsub` package
2. Update ADR-25 to reference new structure
3. Create migration guide
4. Add deprecation notices in godoc

### Phase 3: Implement New Features

1. Implement `pubsub/channel/` broker
2. Add README to each subpackage
3. Add integration tests

### Phase 4: Deprecation (After 2 Major Versions)

1. Mark `message/pubsub.go` as deprecated (v1.0.0)
2. Mark `message/broker/` as deprecated (v1.0.0)
3. Remove in v3.0.0 (after 2 major versions)

## Implementation Details

### Package pubsub/pubsub.go

```go
package pubsub

import (
    "context"
    "time"

    "github.com/fxsml/gopipe"
    "github.com/fxsml/gopipe/channel"
    "github.com/fxsml/gopipe/message"
)

// Sender sends messages to topics.
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver receives messages from topics.
type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Broker combines sending and receiving capabilities.
type Broker interface {
    Sender
    Receiver
}

// Publisher wraps a Sender with batching and grouping.
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

// Subscriber wraps a Receiver with polling.
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) <-chan *message.Message
}

// PublisherConfig configures Publisher behavior.
type PublisherConfig struct {
    MaxBatchSize int
    MaxDuration  time.Duration
    Concurrency  int
    Timeout      time.Duration
    Retry        *gopipe.RetryConfig
    Recover      bool
}

// NewPublisher creates a Publisher that batches messages by topic.
func NewPublisher(
    sender Sender,
    route func(msg *message.Message) string,
    config PublisherConfig,
) Publisher {
    // Implementation moved from message/pubsub.go
}

// SubscriberConfig configures Subscriber behavior.
type SubscriberConfig struct {
    Concurrency int
    Timeout     time.Duration
    Retry       *gopipe.RetryConfig
    Recover     bool
}

// NewSubscriber creates a Subscriber that polls for messages.
func NewSubscriber(
    receiver Receiver,
    config SubscriberConfig,
) Subscriber {
    // Implementation moved from message/pubsub.go
}
```

### Package pubsub/memory/broker.go

```go
package memory

import (
    "context"
    "time"

    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pubsub"
)

// Config configures the in-memory broker.
type Config struct {
    CloseTimeout   time.Duration
    SendTimeout    time.Duration
    ReceiveTimeout time.Duration
    BufferSize     int
}

// NewBroker creates a new in-memory broker.
func NewBroker(config Config) pubsub.Broker {
    // Implementation moved from message/broker/broker.go
}
```

### Package pubsub/channel/broker.go (New!)

```go
package channel

import (
    "context"
    "sync"

    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pubsub"
)

type channelBroker struct {
    mu     sync.RWMutex
    topics map[string]chan *message.Message
}

// NewBroker creates a channel-based broker.
// Messages are streamed through Go channels with zero-copy.
func NewBroker() pubsub.Broker {
    return &channelBroker{
        topics: make(map[string]chan *message.Message),
    }
}

func (b *channelBroker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
    b.mu.RLock()
    ch, ok := b.topics[topic]
    b.mu.RUnlock()

    if !ok {
        b.mu.Lock()
        ch, ok = b.topics[topic]
        if !ok {
            ch = make(chan *message.Message, 100)
            b.topics[topic] = ch
        }
        b.mu.Unlock()
    }

    for _, msg := range msgs {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case ch <- msg:
        }
    }

    return nil
}

func (b *channelBroker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
    b.mu.RLock()
    ch, ok := b.topics[topic]
    b.mu.RUnlock()

    if !ok {
        b.mu.Lock()
        ch, ok = b.topics[topic]
        if !ok {
            ch = make(chan *message.Message, 100)
            b.topics[topic] = ch
        }
        b.mu.Unlock()
    }

    var msgs []*message.Message
    for {
        select {
        case <-ctx.Done():
            return msgs, ctx.Err()
        case msg := <-ch:
            msgs = append(msgs, msg)
            // Batch messages if more are available
            if len(msgs) >= 100 {
                return msgs, nil
            }
        default:
            if len(msgs) > 0 {
                return msgs, nil
            }
            // Wait for at least one message
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case msg := <-ch:
                return []*message.Message{msg}, nil
            }
        }
    }
}
```

## Breaking Changes

**None** - Migration is backward-compatible:
1. Keep existing `message/pubsub.go` with type aliases
2. Keep existing `message/broker/` with forwarding functions
3. Deprecate old paths over 2 major versions

## Alternatives Considered

### Alternative 1: Keep Current Structure

**Rejected because:**
- Confusing naming (`broker.NewBroker`)
- Mixed concerns (`message/pubsub.go`)
- Poor organization

### Alternative 2: Subpackage Under `message/`

```
message/
├── pubsub/
│   ├── memory/
│   ├── http/
│   └── io/
```

**Rejected because:**
- Still couples pub/sub to core messaging
- `message/pubsub` is redundant
- Top-level `pubsub` package is clearer

### Alternative 3: Implementations Under `broker/`

```
broker/
├── memory/
├── http/
└── io/
```

**Rejected because:**
- "broker" is too generic
- Doesn't clearly indicate pub/sub nature
- Harder to find

## References

- [ADR-25: Interface Broker](../adr-25_broker.md)
- [Critical Analysis: Message Design](../critical-analysis-message-design.md)
- [Example: broker](../../examples/broker/)
