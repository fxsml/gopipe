# Topics and Destinations

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Destination** | Logical routing name (matches publisher name) | `"shipments"`, `"notifications"` |
| **Subject** | CloudEvents attribute for topic/category | `"order.created"`, `"user.123"` |

## Design

### Subscriber: Topic at Subscribe Time

Subscribers need topics at runtime for dynamic subscription patterns:

```go
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) (<-chan *Message, error)
}
```

**Why runtime topic?**
- Leader election: subscribe only when leader
- Dynamic scaling: subscribe to partition-specific topics
- Multi-tenant: subscribe to tenant-specific topics

```go
// Dynamic subscription based on leadership
election.OnBecomeLeader(func() {
    subscriber.Subscribe(ctx, "orders")
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

## CloudEvents Mapping

| gopipe | CloudEvents | Notes |
|--------|-------------|-------|
| `Subject` | `subject` | Optional, describes topic/category |
| `Destination` | N/A | gopipe extension for routing (logical name) |
| `Source` | `source` | Required, origin URI |
| `Type` | `type` | Required, event type |

## Summary

| Component | Concept | When | Example |
|-----------|---------|------|---------|
| Subscriber | `topic` parameter | Subscribe time | `"orders"` |
| Message | `Destination` attribute | Message creation | `"shipments"` |
| Publisher | name (matches Destination) | Registration | `"shipments"` |
| Adapter | broker config | Construction | URL, topic mapping |
