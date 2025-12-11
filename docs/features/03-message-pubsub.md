# Feature: Publisher/Subscriber and Broker Implementations

**Package:** `message`, `message/broker`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0010](../adr/0010-pubsub-package-structure.md) - Pub/Sub Package Structure

## Summary

Implements pub/sub abstractions (Sender, Receiver, Publisher, Subscriber) and three broker implementations: channel-based in-memory, HTTP with CloudEvents, and IO for debugging.

## Core Interfaces

```go
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*Message) error
}

type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*Message, error)
}

type Broker interface {
    Sender
    Receiver
}
```

## Publisher & Subscriber

### Publisher
Batches messages and publishes via Sender with configurable batching:

```go
pub := message.NewPublisher(broker, message.PublisherConfig{
    MaxBatchSize: 100,
    MaxDuration:  time.Second,
})

// Publish returns ack channel
ack := pub.Publish(ctx, msgs)
<-ack // Wait for publish completion
```

### Subscriber
Subscribes to topics and converts Receiver polling into channel output:

```go
sub := message.NewSubscriber(broker, message.SubscriberConfig{
    Concurrency: 5,
    PollInterval: 100 * time.Millisecond,
})

msgs := sub.Subscribe(ctx, "orders.created")
// Merge multiple topics if needed
allMsgs := channel.Merge(
    sub.Subscribe(ctx, "orders.created"),
    sub.Subscribe(ctx, "payments.completed"),
)
```

## Broker Implementations

### 1. Channel Broker (In-Memory)
```go
broker := broker.NewChannelBroker()
```
- Thread-safe in-memory pub/sub
- Uses Go channels internally
- Perfect for testing and local development

### 2. HTTP Broker (CloudEvents)
```go
// Sender only (webhook publisher)
sender := broker.NewHTTPSender(httpClient, broker.HTTPSenderConfig{
    DefaultURL: "https://api.example.com/webhooks",
})

// Receiver only (webhook receiver)
receiver := broker.NewHTTPReceiver()
handler := receiver.Handler() // http.Handler for your server
```
- Implements CloudEvents HTTP Protocol Binding v1.0.2
- Binary and structured content modes
- Batching support

### 3. IO Broker (Debug/Logging)
```go
broker := broker.NewIOBroker(os.Stdout, os.Stdin)
```
- JSONL format (one CloudEvent per line)
- For debugging, logging, and bridging
- Topic filtering on receive
- **Not for production use**

## Files Changed

- `message/publisher.go` - Publisher with batching
- `message/publisher_test.go` - Publisher tests
- `message/subscriber.go` - Subscriber implementation
- `message/subscriber_test.go` - Subscriber tests
- `message/broker/channel.go` - Channel broker implementation
- `message/broker/http.go` - HTTP CloudEvents broker
- `message/broker/io.go` - IO broker for debugging
- `message/broker/compat_test.go` - Broker compatibility tests

## Usage Example

```go
// Setup
broker := broker.NewChannelBroker()
pub := message.NewPublisher(broker, message.PublisherConfig{})
sub := message.NewSubscriber(broker, message.SubscriberConfig{})

// Subscribe
orderMsgs := sub.Subscribe(ctx, "orders")

// Publish
msgs := []*message.Message{{Data: []byte("order1")}}
pub.Publish(ctx, msgs)

// Consume
for msg := range orderMsgs {
    process(msg)
}
```

## Related Features

- [01-channel-groupby](01-channel-groupby.md) - Used for publisher batching
- [06-message-cloudevents](06-message-cloudevents.md) - CloudEvents serialization
- [07-message-multiplex](07-message-multiplex.md) - Topic-based routing
