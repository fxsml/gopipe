# Feature: Multiplex Topic Routing

**Package:** `message/multiplex`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0012](../adr/0012-multiplex-pubsub.md) - Multiplex Pub/Sub

## Summary

Provides topic-based routing for Sender and Receiver, enabling multiple publishers/subscribers to share a single broker connection while maintaining topic isolation.

## Core Components

### Multiplex Sender
Routes messages to different Senders based on topic:

```go
func NewSender(routes map[string]message.Sender, defaultSender message.Sender) message.Sender
```

**Example:**
```go
orderBroker := broker.NewChannelBroker()
paymentBroker := broker.NewChannelBroker()

sender := multiplex.NewSender(map[string]message.Sender{
    "orders":   orderBroker,
    "payments": paymentBroker,
}, nil)

// Routes to orderBroker
sender.Send(ctx, "orders", orderMsgs)

// Routes to paymentBroker
sender.Send(ctx, "payments", paymentMsgs)
```

### Multiplex Receiver
Routes topic subscriptions to different Receivers:

```go
func NewReceiver(routes map[string]message.Receiver, defaultReceiver message.Receiver) message.Receiver
```

**Example:**
```go
receiver := multiplex.NewReceiver(map[string]message.Receiver{
    "orders":   orderBroker,
    "payments": paymentBroker,
}, nil)

// Receives from orderBroker
orderMsgs, _ := receiver.Receive(ctx, "orders")

// Receives from paymentBroker
paymentMsgs, _ := receiver.Receive(ctx, "payments")
```

## Use Cases

### 1. Multi-Broker Fan-Out
Publish to different brokers based on topic:

```go
sender := multiplex.NewSender(map[string]message.Sender{
    "critical":   productionBroker,
    "analytics":  analyticsQueue,
    "audit":      auditLog,
}, nil)

pub := message.NewPublisher(sender, message.PublisherConfig{})
```

### 2. Topic-Based Broker Selection
Route high-volume vs. low-latency topics to different brokers:

```go
sender := multiplex.NewSender(map[string]message.Sender{
    "realtime":   lowLatencyBroker,
    "batch":      highThroughputQueue,
}, defaultBroker)
```

### 3. Testing and Mocking
Route specific topics to test doubles:

```go
receiver := multiplex.NewReceiver(map[string]message.Receiver{
    "test.orders": mockBroker,
}, realBroker)
```

### 4. Environment-Specific Routing
Different brokers for different environments embedded in topic:

```go
sender := multiplex.NewSender(map[string]message.Sender{
    "prod.orders": prodBroker,
    "dev.orders":  devBroker,
}, nil)
```

## Default Sender/Receiver

When a topic doesn't match any route, the default is used:

```go
sender := multiplex.NewSender(routes, defaultSender)
// Unknown topics go to defaultSender
```

If no default is provided, unmatched topics return an error.

## Pattern Matching

Current implementation uses exact topic matching. Topic patterns can be implemented by wrapping with custom routing logic:

```go
func topicRouter(topic string, senders map[string]message.Sender) message.Sender {
    // Custom pattern matching
    if strings.HasPrefix(topic, "orders.") {
        return senders["orders"]
    }
    return senders["default"]
}
```

## Files Changed

- `message/multiplex/multiplex.go` - Multiplex sender and receiver
- `message/multiplex/multiplex_test.go` - Comprehensive tests

## Usage Example

```go
// Setup multiple brokers
inMemory := broker.NewChannelBroker()
http := broker.NewHTTPSender(client, broker.HTTPSenderConfig{
    DefaultURL: "https://webhooks.example.com",
})

// Create multiplex sender
sender := multiplex.NewSender(map[string]message.Sender{
    "local":  inMemory,
    "remote": http,
}, inMemory) // Default to in-memory

// Use with publisher
pub := message.NewPublisher(sender, message.PublisherConfig{})

// Messages route based on topic attribute
localMsgs := []*message.Message{{
    Data: []byte("test"),
    Attributes: message.Attributes{
        message.AttrSubject: "local",
    },
}}
pub.Publish(ctx, localMsgs) // -> inMemory

remoteMsgs := []*message.Message{{
    Data: []byte("webhook"),
    Attributes: message.Attributes{
        message.AttrSubject: "remote",
    },
}}
pub.Publish(ctx, remoteMsgs) // -> http
```

## Related Features

- [03-message-pubsub](03-message-pubsub.md) - Publisher/Subscriber use multiplex for routing
