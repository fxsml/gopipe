# Pubsub Package

The `pubsub` package provides publish-subscribe messaging primitives for building event-driven systems with gopipe.

## Overview

The package provides several key abstractions:

- **Broker**: Core interface combining `Sender` and `Receiver`
- **Publisher**: Batches and sends messages to a broker
- **Subscriber**: Receives messages from a broker as a channel
- **Multiplexing**: Routes messages to different brokers based on configurable logic

## Core Interfaces

### Sender and Receiver

```go
// Sender sends messages to a topic
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver receives messages from a topic
type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Broker combines both interfaces
type Broker interface {
    Sender
    Receiver
}
```

### Publisher

Batches messages and sends them to a broker:

```go
publisher := pubsub.NewPublisher(
    broker,
    pubsub.PublisherConfig{
        MaxBatchSize: 100,
        MaxDuration:  100 * time.Millisecond,
    },
)

msgs := make(chan *message.Message)
done := publisher.Publish(ctx, msgs)

// Send messages
msgs <- message.New([]byte("event"), message.Attributes{
    message.AttrTopic: "orders.created",
})

close(msgs)
<-done // Wait for completion
```

### Subscriber

Receives messages from multiple topics as a single channel:

```go
// Subscribe to a single topic
subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
msgChan := subscriber.Subscribe(ctx, "orders.created")

for msg := range msgChan {
    // Process messages from the topic
    fmt.Println(string(msg.Data))
}

// Subscribe to multiple topics and merge channels
orders := subscriber.Subscribe(ctx, "orders.created")
updates := subscriber.Subscribe(ctx, "orders.updated")
allMsgs := channel.Merge(orders, updates)

for msg := range allMsgs {
    // Process messages from both topics
    fmt.Println(string(msg.Data))
}
```

Each call to Subscribe creates an independent subscription with its own polling goroutine. Use `channel.Merge` to combine multiple topic subscriptions.

## Multiplexing

Route messages to different brokers based on topic patterns:

### Basic Usage

```go
// Create brokers
memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
natsBroker := createNATSBroker()

// Route internal topics to memory, everything else to NATS
selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
multiplexSender := pubsub.NewMultiplexSender(selector, natsBroker)

// Use with publisher
publisher := pubsub.NewPublisher(
    multiplexSender,
    pubsub.PublisherConfig{},
)
```

### Pattern Matching

Support for wildcard patterns:

```go
selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Pattern: "internal.*", Sender: memoryBroker},      // Single segment
    {Pattern: "audit.**", Sender: auditBroker},         // Multi-segment
    {Pattern: "orders.*.created", Sender: natsBroker},  // Wildcard in middle
})

multiplexSender := pubsub.NewMultiplexSender(selector, defaultBroker)
```

**Pattern Syntax**:
- `*` matches a single segment: `orders.*` matches `orders.created` but not `orders.us.created`
- `**` matches multiple segments: `audit.**` matches `audit.log`, `audit.us.log`, `audit.us.2024.log`
- Exact match when no wildcards: `orders.created` matches only `orders.created`

### Chained Selectors

Combine multiple routing strategies:

```go
selector := pubsub.ChainSenderSelectors(
    pubsub.PrefixSenderSelector("audit", auditBroker),
    pubsub.PrefixSenderSelector("internal", memoryBroker),
    customSelector,
)

multiplexSender := pubsub.NewMultiplexSender(selector, fallbackBroker)
```

### Custom Selectors

Write custom routing logic:

```go
customSelector := func(topic string) pubsub.Sender {
    if strings.Contains(topic, ".priority.") {
        return priorityBroker
    }
    if time.Now().Hour() < 9 {  // Off-hours
        return batchBroker
    }
    return nil  // Use fallback
}

multiplexSender := pubsub.NewMultiplexSender(customSelector, defaultBroker)
```

## CloudEvents Package

The `pubsub/cloudevents` package provides CloudEvents v1.0.2 serialization and deserialization for gopipe messages. It supports both the HTTP protocol binding and JSON format specifications.

```go
import "github.com/fxsml/gopipe/pubsub/cloudevents"

// Convert message to CloudEvent
msg := message.New([]byte(`{"order":"123"}`), message.Attributes{
    message.AttrID:     "evt-001",
    message.AttrSource: "orders-service",
    message.AttrType:   "order.created",
})
event := cloudevents.FromMessage(msg, "orders.created", "default-source")

// Marshal to JSON
data, _ := json.Marshal(event)

// Unmarshal from JSON
var parsed cloudevents.Event
json.Unmarshal(data, &parsed)

// Convert back to message
msg, topic, _ := cloudevents.ToMessage(&parsed)
```

The package handles:
- Standard CloudEvents attributes (id, source, specversion, type, subject, time, datacontenttype)
- gopipe extension attributes (topic, correlationid, deadline)
- JSON data (stored as JSON value per spec)
- Binary data (Base64-encoded in `data_base64` field)
- Custom extension attributes

## Broker Implementations

### Channel Broker

Fast, in-process broker using Go channels for testing and local development:

```go
broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{
    CloseTimeout: 5 * time.Second,
    SendTimeout:  time.Second,
    BufferSize:   100,
})
```

### IO Broker

Stream-based broker using `io.Reader`/`io.Writer` with CloudEvents JSON format:

```go
// File-based - messages stored as CloudEvents JSON Lines (JSONL)
file, _ := os.Create("events.jsonl")
broker := pubsub.NewIOBroker(file, file, pubsub.IOConfig{
    Source: "gopipe://myapp", // Optional: CloudEvents source identifier
})

// Pipe-based (IPC)
pr, pw := io.Pipe()
broker := pubsub.NewIOBroker(pr, pw, pubsub.IOConfig{})
```

**CloudEvents JSON Format**

The IO broker uses [CloudEvents JSON format](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/formats/json-format.md) for message serialization. Each message is encoded as a single-line JSON object (JSONL) containing:
- Standard CloudEvents attributes (id, source, specversion, type, subject, time, datacontenttype)
- gopipe extension attributes (topic, correlationid, deadline)
- Data payload (as JSON value, string, or Base64-encoded binary)

### HTTP Broker

Webhook-style broker using HTTP with CloudEvents support:

```go
// Sender (POST to webhook) - Binary mode (default)
sender := pubsub.NewHTTPSender("https://example.com/webhook", pubsub.HTTPConfig{
    CloudEventsMode: pubsub.CloudEventsBinary,  // Default
    Headers: map[string]string{
        "Authorization": "Bearer token",
    },
})

// Structured content mode (full CloudEvent JSON)
sender := pubsub.NewHTTPSender("https://example.com/webhook", pubsub.HTTPConfig{
    CloudEventsMode: pubsub.CloudEventsStructured,
})

// Batch content mode (array of CloudEvents)
sender := pubsub.NewHTTPSender("https://example.com/webhook", pubsub.HTTPConfig{
    CloudEventsMode: pubsub.CloudEventsBatch,
})

// Receiver (accept webhook POSTs) - auto-detects CloudEvents mode
receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, bufferSize)
```

**CloudEvents Support**

The HTTP broker implements the [CloudEvents HTTP Protocol Binding v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md) specification with three content modes:

- **Binary Mode** (default): Event data in HTTP body, attributes as `ce-` prefixed headers
- **Structured Mode**: Full CloudEvent JSON in body with `application/cloudevents+json` content type
- **Batch Mode**: Array of CloudEvents in body with `application/cloudevents-batch+json` content type

The receiver automatically detects the CloudEvents mode from the Content-Type header or presence of `ce-` headers. All standard CloudEvents attributes (id, source, specversion, type, subject, time, datacontenttype) are supported, along with gopipe extension attributes (topic, correlationid, deadline).


## Common Patterns

### Pattern 1: Environment-Based Routing

```go
func createBroker() pubsub.Sender {
    if os.Getenv("ENV") == "development" {
        return pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
    }

    memory := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
    nats := createNATSBroker()

    selector := pubsub.PrefixSenderSelector("internal", memory)
    return pubsub.NewMultiplexSender(selector, nats)
}
```

### Pattern 2: Audit Trail with Separate Broker

```go
mainBroker := createMainBroker()
auditBroker := createAuditBroker()

selector := pubsub.PrefixSenderSelector("audit", auditBroker)
multiplex := pubsub.NewMultiplexSender(selector, mainBroker)

publisher := pubsub.NewPublisher(multiplex, config)
```

### Pattern 3: Performance Optimization

```go
// Fast in-memory for high-throughput internal events
memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{
    BufferSize: 10000,
})

// Durable external broker for critical events
kafkaBroker := createKafkaBroker()

selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Pattern: "internal.**", Sender: memoryBroker},
    {Pattern: "metrics.**", Sender: memoryBroker},
})

multiplex := pubsub.NewMultiplexSender(selector, kafkaBroker)
```

### Pattern 4: Region-Based Routing

```go
usEastBroker := createNATSBroker("nats://us-east.example.com")
usWestBroker := createNATSBroker("nats://us-west.example.com")
euBroker := createNATSBroker("nats://eu.example.com")

selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Pattern: "events.us-east.*", Sender: usEastBroker},
    {Pattern: "events.us-west.*", Sender: usWestBroker},
    {Pattern: "events.eu.*", Sender: euBroker},
})

multiplex := pubsub.NewMultiplexSender(selector, usEastBroker) // Default
```

## Testing

### Using Mock Brokers

```go
func TestMessageProcessing(t *testing.T) {
    broker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})

    publisher := pubsub.NewPublisher(
        broker,
        pubsub.PublisherConfig{MaxBatchSize: 10},
    )

    ctx := context.Background()
    msgs := make(chan *message.Message, 1)

    msgs <- message.New([]byte("test"), message.Attributes{
        message.AttrTopic: "test.topic",
    })
    close(msgs)

    <-publisher.Publish(ctx, msgs)

    // Verify message was sent
    received, _ := broker.Receive(ctx, "test.topic")
    if len(received) != 1 {
        t.Errorf("expected 1 message, got %d", len(received))
    }
}
```

### Testing Multiplex Routing

```go
func TestMultiplexRouting(t *testing.T) {
    memoryBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})
    natsBroker := pubsub.NewChannelBroker(pubsub.ChannelBrokerConfig{})

    selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
    multiplex := pubsub.NewMultiplexSender(selector, natsBroker)

    ctx := context.Background()
    msg := message.New([]byte("test"), message.Attributes{})

    // Should route to memory broker
    multiplex.Send(ctx, "internal.events", []*message.Message{msg})

    internalMsgs, _ := memoryBroker.Receive(ctx, "internal.events")
    if len(internalMsgs) != 1 {
        t.Error("expected message in memory broker")
    }

    // Should route to NATS broker (fallback)
    multiplex.Send(ctx, "external.api", []*message.Message{msg})

    externalMsgs, _ := natsBroker.Receive(ctx, "external.api")
    if len(externalMsgs) != 1 {
        t.Error("expected message in NATS broker")
    }
}
```

## Performance Considerations

1. **Batching**: Configure `MaxBatchSize` and `MaxDuration` based on throughput needs
2. **Buffer Sizes**: Increase buffer sizes for high-throughput scenarios
3. **Selector Performance**: Keep selector functions O(1) or O(n) with small n
4. **Memory Broker**: Use for internal, high-throughput messages
5. **Pattern Matching**: Prefer simple patterns (`*`) over complex ones (`**`) for best performance

## Error Handling

```go
// Publisher errors
done := publisher.Publish(ctx, msgs)
<-done  // Blocks until all messages sent or error

// Sender errors
err := sender.Send(ctx, topic, msgs)
if err != nil {
    log.Printf("Failed to send: %v", err)
}

// Receiver errors
msgs, err := receiver.Receive(ctx, topic)
if err != nil {
    log.Printf("Failed to receive: %v", err)
}
```

## See Also

- [Multiplex Example](../examples/multiplex-pubsub/main.go)
- [Broker Example](../examples/broker/main.go)
- [ADR-0010: Pubsub Package Structure](../docs/adr/0010-pubsub-package-structure.md)
- [ADR-0012: Multiplex Publisher/Subscriber](../docs/adr/0012-multiplex-pubsub.md)
