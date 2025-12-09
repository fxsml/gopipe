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
    pubsub.RouteBySubject(),
    pubsub.PublisherConfig{
        MaxBatchSize: 100,
        MaxDuration:  100 * time.Millisecond,
    },
)

msgs := make(chan *message.Message)
done := publisher.Publish(ctx, msgs)

// Send messages
msgs <- message.New([]byte("event"), message.Properties{
    message.PropSubject: "orders.created",
})

close(msgs)
<-done // Wait for completion
```

### Subscriber

Receives messages as a channel:

```go
subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
msgChan := subscriber.Subscribe(ctx, "orders.created")

for msg := range msgChan {
    // Process message
    fmt.Println(string(msg.Payload))
}
```

## Routing

### Routing Functions

Determine how messages map to topics:

```go
// Route by subject property
RouteBySubject() RouteFunc

// Route by specific property
RouteByProperty(key string) RouteFunc

// Route all to static topic
RouteStatic(topic string) RouteFunc

// Route using format string
RouteByFormat(format string, keys ...string) RouteFunc
```

Example:

```go
// Route by subject
pubsub.RouteBySubject()
// Message with PropSubject="orders.created" → topic "orders.created"

// Route by custom property
pubsub.RouteByProperty("region")
// Message with Properties{"region": "us-east"} → topic "us-east"

// Route to static topic
pubsub.RouteStatic("all-events")
// All messages → topic "all-events"

// Route by format
pubsub.RouteByFormat("events/%s/%s", "type", "region")
// Message with Properties{"type": "order", "region": "us"} → topic "events/order/us"
```

## Multiplexing

Route messages to different brokers based on topic patterns:

### Basic Usage

```go
// Create brokers
memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
natsBroker := createNATSBroker()

// Route internal topics to memory, everything else to NATS
selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
multiplexSender := pubsub.NewMultiplexSender(selector, natsBroker)

// Use with publisher
publisher := pubsub.NewPublisher(
    multiplexSender,
    pubsub.RouteBySubject(),
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

## Broker Implementations

### In-Memory Broker

Fast, non-durable broker for testing and local development:

```go
broker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{
    CloseTimeout: 5 * time.Second,
    SendTimeout:  time.Second,
    BufferSize:   100,
})
```

### IO Broker

Stream-based broker using `io.Reader`/`io.Writer`:

```go
// File-based
file, _ := os.Create("events.jsonl")
broker := pubsub.NewIOBroker(file, file, pubsub.IOConfig{})

// Pipe-based (IPC)
pr, pw := io.Pipe()
broker := pubsub.NewIOBroker(pr, pw, pubsub.IOConfig{})
```

### HTTP Broker

Webhook-style broker using HTTP:

```go
// Sender (POST to webhook)
sender := pubsub.NewHTTPSender("https://example.com/webhook", pubsub.HTTPConfig{
    Headers: map[string]string{
        "Authorization": "Bearer token",
    },
})

// Receiver (accept webhook POSTs)
receiver := pubsub.NewHTTPReceiver(pubsub.HTTPConfig{}, bufferSize)
```

### Channel Broker

Broker backed by Go channels (for testing):

```go
sendChan := make(chan TopicMessage, 100)
recvChan := make(chan TopicMessage, 100)
broker := pubsub.NewChannelBroker(sendChan, recvChan)
```

## Common Patterns

### Pattern 1: Environment-Based Routing

```go
func createBroker() pubsub.Sender {
    if os.Getenv("ENV") == "development" {
        return pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
    }

    memory := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
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

publisher := pubsub.NewPublisher(multiplex, pubsub.RouteBySubject(), config)
```

### Pattern 3: Performance Optimization

```go
// Fast in-memory for high-throughput internal events
memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{
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
    broker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

    publisher := pubsub.NewPublisher(
        broker,
        pubsub.RouteBySubject(),
        pubsub.PublisherConfig{MaxBatchSize: 10},
    )

    ctx := context.Background()
    msgs := make(chan *message.Message, 1)

    msgs <- message.New([]byte("test"), message.Properties{
        message.PropSubject: "test.topic",
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
    memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
    natsBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})

    selector := pubsub.PrefixSenderSelector("internal", memoryBroker)
    multiplex := pubsub.NewMultiplexSender(selector, natsBroker)

    ctx := context.Background()
    msg := message.New([]byte("test"), message.Properties{})

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
