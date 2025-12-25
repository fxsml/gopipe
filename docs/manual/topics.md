# Topics and Routing

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Subscriber** | Subscriber name (set by engine on ingress) | `"order-events"`, `"payment-stream"` |
| **Publisher** | Publisher name (set by handler for egress) | `"shipments"`, `"notifications"` |
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

When messages arrive through a subscriber, the engine sets the `Subscriber` attribute to the subscriber name. This provides traceability for where messages entered the engine.

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

### Publisher: Publisher Attribute in Message

Handlers set the `Publisher` attribute to route messages:

```go
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) error
}
```

**Why publisher in message?**
- Handlers decide routing, not infrastructure
- Single engine can route to multiple publishers
- Publisher is a handler concern, not a wiring concern

```go
// Handler determines publisher (logical name, not URL)
func handleOrder(ctx context.Context, order OrderCreated) ([]*Message, error) {
    return []*Message{
        message.New(OrderShipped{...}, message.Attributes{
            Subject:   "order.shipped",
            Publisher: "shipments",  // logical name, matches registered publisher
        }),
    }, nil
}
```

### Publisher Attribute = Registered Publisher Name

The `Publisher` attribute is a **logical routing name** that matches the publisher registration:

```go
// Publishers registered by logical name
engine.AddPublisher("shipments", kafkaPublisher)
engine.AddPublisher("notifications", natsPublisher)

// Handler sets Publisher attribute to route to specific publisher
message.New(event, Attributes{Publisher: "shipments"})
```

The publisher/adapter knows the actual broker details (host, port, topic). The message only carries logical routing intent.

### Loopback

Use the `Loopback` constant to route messages back through the engine:

```go
// Route message back through engine for further processing
message.New(event, Attributes{Publisher: message.Loopback})
```

### Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Engine                                    │
│                                                                  │
│  Subscriber.Subscribe(ctx, "orders")                             │
│       ↓                                                          │
│  [messages with Subscriber attribute set]                        │
│       ↓                                                          │
│  Engine routes by Publisher → registered Publisher               │
│       ↓                                                          │
│  Publisher/Adapter sends to actual broker                        │
└─────────────────────────────────────────────────────────────────┘
```

**Adapter configuration (not in message):**
- Broker URL: `kafka://broker:9092`
- Topic mapping: publisher `"shipments"` → topic `"prod.shipments.v1"`
- Authentication, TLS, etc.

## CloudEvents Mapping

| gopipe | CloudEvents | Notes |
|--------|-------------|-------|
| `Subscriber` | N/A | gopipe extension for ingress tracing |
| `Publisher` | N/A | gopipe extension for egress routing |
| `Subject` | `subject` | Optional, describes topic/category |
| `Source` | `source` | Required, origin URI (CloudEvents) |
| `Type` | `type` | Required, event type |

## Summary

| Component | Concept | When | Example |
|-----------|---------|------|---------|
| Subscriber | name + topic | Registration + Subscribe | `"order-events"` + `"orders"` |
| Message (ingress) | `Subscriber` attribute | Set by engine | `"order-events"` |
| Message (egress) | `Publisher` attribute | Set by handler | `"shipments"` |
| Publisher | name (matches Publisher attr) | Registration | `"shipments"` |
| Adapter | broker config | Construction | URL, topic mapping |
