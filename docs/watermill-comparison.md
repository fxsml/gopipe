# Watermill vs gopipe: Messaging Library Comparison

## Executive Summary

**Watermill** is a purpose-built event-driven messaging library focused on pub/sub patterns with multiple broker integrations.

**gopipe** is a general-purpose pipelining library optimized for composable data transformations with optional messaging support.

## Core Philosophy

### Watermill
- **Goal**: "Make communication with messages as easy as HTTP routers"
- **Focus**: Event-driven applications, message broker abstraction
- **Paradigm**: Handler-based routing, pub/sub messaging

### gopipe
- **Goal**: Orchestrate complex data pipelines with composable pipes
- **Focus**: Generic data processing, channel orchestration
- **Paradigm**: Functional composition, pipeline orchestration

## Message Type Comparison

### Watermill Message (Non-Generic)

```go
type Message struct {
    UUID     string           // Unique identifier
    Metadata Metadata         // map[string]string
    Data  Data          // []byte

    // Internal ack channels
    ack      chan struct{}
    noAck    chan struct{}
    ctx      context.Context
}
```

**Key characteristics:**
- Non-generic, always `[]byte` data
- Simple UUID string (not typed)
- Metadata is `map[string]string` only
- Context embedded in message
- Channel-based ack/nack (signals via closing channels)

### gopipe Message (Generic)

```go
type Message[T any] struct {
    data    T                  // Generic typed data
    attributes *Attributes        // Thread-safe map[string]any
    a          *acking            // Callback-based acking
}

type acking struct {
    mu               sync.Mutex
    ack              func()
    nack             func(error)
    ackType          ackType
    ackCount         int
    expectedAckCount int          // Multi-stage support
}
```

**Key characteristics:**
- Generic typed data (any type `T`)
- Rich attributes with typed accessors (ID, CorrelationID, CreatedAt, etc.)
- Attributes are `map[string]any` (not just strings)
- Callback-based ack/nack with error propagation
- Multi-stage acknowledgment support
- No context embedding (uses deadline instead)

## Acknowledgment Mechanisms

### Watermill: Channel-Based

```go
// Ack closes the ack channel
func (m *Message) Ack() bool {
    m.ackMutex.Lock()
    defer m.ackMutex.Unlock()

    if m.ackSentType == noAckSent {
        close(m.ack)
        m.ackSentType = ackSent
        return true
    }
    return false
}

// Wait for acknowledgment
select {
case <-msg.Acked():
    // Message acknowledged
case <-msg.Nacked():
    // Message rejected
}
```

**Pros:**
- Enables waiting on ack/nack via channels
- No external dependency injection

**Cons:**
- No error information on nack
- Single-stage only
- Cannot customize ack behavior per message

### gopipe: Callback-Based

```go
// Ack calls the callback when all stages complete
func (m *Message[T]) Ack() bool {
    m.a.mu.Lock()
    defer m.a.mu.Unlock()

    m.a.ackCount++
    if m.a.ackCount < m.a.expectedAckCount {
        return true  // Not all stages done
    }

    m.a.ack()  // Execute callback
    m.a.ackType = ackTypeAck
    return true
}

// Nack immediately rejects with error
func (m *Message[T]) Nack(err error) bool {
    // ...
    m.a.nack(err)  // Pass error to callback
    m.a.ackType = ackTypeNack
    return true
}
```

**Pros:**
- Error propagation on nack
- Multi-stage pipeline support
- Flexible callback injection per message
- Integrates with any broker's ack mechanism

**Cons:**
- Cannot wait on acknowledgment completion
- Requires callback injection

## Simple Examples

### Watermill: Publisher/Subscriber

```go
// Publisher (simplified)
publisher, _ := gochannel.NewGoChannel(
    gochannel.Config{},
    watermill.NewStdLogger(false, false),
)

msg := message.NewMessage(watermill.NewUUID(), []byte("hello"))
publisher.Publish("topic", msg)

// Subscriber
messages, _ := publisher.Subscribe(context.Background(), "topic")
for msg := range messages {
    fmt.Println(string(msg.Data))
    msg.Ack()
}
```

### gopipe: Pipeline with Messaging

```go
// Producer
in := channel.FromValues(
    message.New([]byte("hello"),
        message.WithID[[]byte]("msg-1"),
        message.WithAcking[[]byte](
            func() { fmt.Println("✓ acked") },
            func(err error) { fmt.Printf("✗ nacked: %v\n", err) },
        ),
    ),
)

// Processing pipe
pipe := gopipe.NewTransformPipe(
    func(ctx context.Context, msg *message.Message[[]byte]) (*message.Message[[]byte], error) {
        // Process
        msg.Ack()
        return msg, nil
    },
)

results := pipe.Start(context.Background(), in)
<-channel.Sink(results, func(msg *message.Message[[]byte]) {
    fmt.Println(string(msg.Data()))
})
```

## Routing and Handlers

### Watermill Router

```go
router, _ := message.NewRouter(message.RouterConfig{}, logger)

router.AddHandler(
    "handler-name",
    "input-topic",
    subscriber,
    "output-topic",
    publisher,
    func(msg *message.Message) ([]*message.Message, error) {
        // Process and return messages
        return []*message.Message{msg}, nil
    },
)

router.Run(context.Background())
```

**Features:**
- Topic-based routing
- Automatic ack/nack on handler return
- Middleware support
- Built-in metrics and tracing

### gopipe feat/pubsub (Removed)

```go
// From feat/pubsub branch (now removed)
router := message.NewRouter(message.RouterConfig{
    Concurrency: 5,
    Timeout:     30 * time.Second,
}, handlers...)

handler := message.NewHandler(
    func(ctx context.Context, data Order) ([]OrderConfirmed, error) {
        // Type-safe processing
        return []OrderConfirmed{{ID: data.ID}}, nil
    },
    func(prop *message.Attributes) bool {
        subject, _ := prop.Subject()
        return subject == "orders.new"
    },
    func(prop *message.Attributes) *message.Attributes {
        // Transform attributes for output
        return prop
    },
)
```

**Features (when it existed):**
- Generic typed handlers
- Property-based matching
- Automatic marshaling/unmarshaling
- Pipe-based execution with full orchestration

**Why removed?** Focused on core pipelining; pub/sub patterns can be built on top.

## Feature Matrix

| Feature | Watermill | gopipe |
|---------|-----------|---------|
| **Message Type** | Non-generic (`[]byte`) | Generic (`T any`) |
| **Metadata** | `map[string]string` | `map[string]any` with typed accessors |
| **Ack/Nack** | Channel-based | Callback-based |
| **Error on Nack** | ❌ No | ✅ Yes |
| **Multi-stage Ack** | ❌ No | ✅ Yes |
| **Context** | Embedded in message | Deadline property + pipe context |
| **Pub/Sub** | ✅ Built-in | ⚠️ Removed from feat/pubsub |
| **Router** | ✅ Built-in | ⚠️ Removed from feat/pubsub |
| **Broker Support** | ✅ 12+ brokers | ❌ DIY |
| **Typed Datas** | ❌ Manual marshal | ✅ Native |
| **Pipeline Composition** | ⚠️ Limited | ✅ Core strength |
| **Middleware** | ✅ Yes | ✅ Yes |
| **Batching** | ❌ No | ✅ Built-in |
| **Concurrency** | ✅ Yes | ✅ Yes |
| **Metrics** | ✅ Built-in | ✅ Built-in |

## Use Case Comparison

### When to use Watermill

✅ **Event-driven microservices**
- Need broker abstraction (Kafka, RabbitMQ, etc.)
- Topic-based routing
- Standard pub/sub patterns

✅ **Message routing**
- Multiple handlers per topic
- Handler-based processing
- Automatic ack/nack

✅ **Simple datas**
- Byte arrays are sufficient
- String metadata only

### When to use gopipe

✅ **Complex data pipelines**
- Multi-stage transformations
- Generic typed processing
- Batch operations

✅ **Custom orchestration**
- Fine-grained concurrency control
- Conditional processing
- Dynamic routing

✅ **Type safety**
- Avoid marshal/unmarshal overhead
- Compile-time type checking
- Rich metadata

✅ **Channel operations**
- Low-level channel wiring
- Fan-in/fan-out
- Stream manipulation

## Critical Analysis

### gopipe Strengths

1. **Type safety**: Generic messages eliminate runtime errors
2. **Flexibility**: Callback-based acking integrates with any broker
3. **Multi-stage support**: Coordinate acknowledgments across pipeline stages
4. **Rich attributes**: Store any type, not just strings
5. **Composability**: Functional pipe composition
6. **Batching**: First-class batch processing support

### gopipe Drawbacks

1. **No broker integration**: Must implement pub/sub adapters manually
2. **Removed router**: feat/pubsub Router was removed, losing convenient message routing
3. **Generic complexity**: Type parameters add cognitive overhead
4. **No built-in pub/sub**: Unlike Watermill, not batteries-included for messaging
5. **Learning curve**: Pipeline composition paradigm vs familiar handler pattern

### Watermill Strengths

1. **Production-ready**: Mature, battle-tested with 12+ broker integrations
2. **Simple**: Non-generic design is easier to understand
3. **Complete**: Router, middleware, pub/sub out of the box
4. **Documentation**: Extensive guides and examples
5. **Community**: Active development and support

### Watermill Drawbacks

1. **Marshal overhead**: Always `[]byte`, requires encoding/decoding
2. **Single-stage ack**: Cannot coordinate multi-stage pipelines
3. **Limited metadata**: String values only
4. **No error context**: Nack doesn't convey error information
5. **Less composable**: Handler-based, not pipeline-oriented

## Next Major Release Recommendations

### Priority 1: Restore Pub/Sub Support

**Problem**: Removed Router and Publisher/Subscriber from feat/pubsub makes gopipe incomplete for messaging use cases.

**Proposal**:
```go
// Restore non-generic interfaces
type Publisher interface {
    Publish(ctx context.Context, topic string, msgs []*Message) error
}

type Subscriber interface {
    Subscribe(ctx context.Context, topic string) <-chan *Message
}

// Non-generic message for pub/sub
type Message = message.Message[[]byte]
```

### Priority 2: Consider Non-Generic Message

**Problem**: Generic `Message[T]` creates friction for pub/sub patterns where `[]byte` is standard.

**Options**:
1. **Dual types**: `Message` (non-generic) + `TypedMessage[T]` (generic)
2. **Alias only**: `Message = Message[[]byte]` for pub/sub, keep generic for pipes
3. **Stay generic**: Accept the trade-off, users cast as needed

**Recommendation**: See separate refactoring proposal (next document).

### Priority 3: Enhance Ack/Nack API

**Add waiting capability**:
```go
func (m *Message[T]) Acknowledged() <-chan error {
    // Returns channel with nil on ack, error on nack
}
```

**Backward compatible**, adds Watermill's waiting pattern.

### Priority 4: Broker Adapters

**Provide reference implementations**:
- Kafka adapter
- RabbitMQ adapter
- NATS adapter

Users can copy/customize rather than starting from scratch.

### Priority 5: Simplify Property Access

**Problem**: Verbose property getters with `(value, bool)` pattern.

**Alternative**:
```go
// Direct access with defaults
msg.Attributes().GetString("key", "default")
msg.Attributes().GetInt64("seq", 0)
```

## Conclusion

**Watermill** excels as a complete, opinionated messaging framework with extensive broker support.

**gopipe** excels as a flexible, type-safe pipelining library with optional messaging capabilities.

The sweet spot for gopipe is **not** to replace Watermill, but to:
1. Provide type-safe pipeline orchestration
2. Offer optional messaging primitives for those who need them
3. Enable users to build custom pub/sub adapters on top

The removal of Router/PubSub from feat/pubsub should be reconsidered with a non-generic message type to compete in the messaging space while maintaining gopipe's pipeline strengths.

## Sources

- [Watermill Documentation](https://watermill.io/docs/)
- [Watermill Message Type](https://pkg.go.dev/github.com/ThreeDotsLabs/watermill/message)
- [Watermill Getting Started](https://watermill.io/learn/getting-started/)
- [Watermill GitHub](https://github.com/ThreeDotsLabs/watermill)
