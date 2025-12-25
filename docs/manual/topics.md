# Topics and Destinations

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Source** | Subscriber name (set by engine on ingress) | `"order-events"`, `"payment-stream"` |
| **Destination** | Publisher name (set by handler for egress) | `"shipments"`, `"notifications"` |
| **Subject** | CloudEvents attribute for topic/category | `"order.created"`, `"user.123"` |

## Design

### Subscriber: Name and Topic

Subscribers are registered by **name** and subscribe to a **topic**:

```go
// Subscriber interface
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) (<-chan *Message, error)
}

// Engine registration with name
engine.AddSubscriber("order-events", subscriber)
```

When messages arrive through a subscriber, the engine sets the `Source` attribute to the subscriber name. This provides traceability for where messages originated.

**Why separate name and topic?**
- **Name**: Identity for logging, metrics, tracing
- **Topic**: What to subscribe to (can be dynamic)

```go
// Dynamic subscription based on leadership
election.OnBecomeLeader(func() {
    subscriber.Subscribe(ctx, "orders")  // topic
})

election.OnLoseLeadership(func() {
    // Context cancellation stops subscription
})
```

### Publisher: Destination in Message

Publishers read destination from message attributes:

```go
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) error
}
```

**Why destination in message?**
- Handlers decide routing, not infrastructure
- Single engine can route to multiple publishers
- Destination is a handler concern, not a wiring concern

```go
// Handler determines destination (logical name, not URL)
func handleOrder(ctx context.Context, order OrderCreated) ([]*Message, error) {
    return []*Message{
        message.New(OrderShipped{...}, message.Attributes{
            Subject:     "order.shipped",
            Destination: "shipments",  // logical name, matches publisher
        }),
    }, nil
}
```

### Destination = Publisher Name

The destination attribute is a **logical routing name** that matches the publisher registration:

```go
// Publisher registered by logical name
engine.AddPublisher("shipments", kafkaPublisher)
engine.AddPublisher("notifications", natsPublisher)

// Handler sets destination to route to specific publisher
message.New(event, Attributes{Destination: "shipments"})
```

The publisher/adapter knows the actual broker details (host, port, topic). The message only carries logical routing intent.

### Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Engine                                    │
│                                                                  │
│  Subscriber.Subscribe(ctx, "orders")                             │
│       ↓                                                          │
│  [messages with Destination attribute]                           │
│       ↓                                                          │
│  Engine routes by Destination → Publisher                        │
│       ↓                                                          │
│  Publisher/Adapter sends to actual broker                        │
└─────────────────────────────────────────────────────────────────┘
```

**Adapter configuration (not in message):**
- Broker URL: `kafka://broker:9092`
- Topic mapping: destination `"shipments"` → topic `"prod.shipments.v1"`
- Authentication, TLS, etc.

### Source = Subscriber Name

The engine sets `Source` on incoming messages to the subscriber name:

```go
// Subscriber registered by name
engine.AddSubscriber("order-events", orderSubscriber)
engine.AddSubscriber("payment-stream", paymentSubscriber)

// Messages from orderSubscriber have Source: "order-events"
// Messages from paymentSubscriber have Source: "payment-stream"
```

This provides symmetry with Destination:
- **Source**: Set by engine on ingress (subscriber name)
- **Destination**: Set by handler on egress (publisher name)

## CloudEvents Mapping

| gopipe | CloudEvents | Notes |
|--------|-------------|-------|
| `Source` | `source` | Subscriber name (set by engine) |
| `Destination` | N/A | gopipe extension for publisher routing |
| `Subject` | `subject` | Optional, describes topic/category |
| `Type` | `type` | Required, event type |

## Summary

| Component | Concept | When | Example |
|-----------|---------|------|---------|
| Subscriber | name + topic | Registration + Subscribe | `"order-events"` + `"orders"` |
| Message (ingress) | `Source` attribute | Set by engine | `"order-events"` |
| Message (egress) | `Destination` attribute | Set by handler | `"shipments"` |
| Publisher | name (matches Destination) | Registration | `"shipments"` |
| Adapter | broker config | Construction | URL, topic mapping |
