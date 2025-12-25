# Topics and Destinations

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Destination** | Where to route a message (URI scheme) | `"kafka://shipments"`, `"nats://events"` |
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
- Single publisher can route to multiple topics
- Destination is a handler concern, not a wiring concern

```go
// Handler determines destination
func handleOrder(ctx context.Context, order OrderCreated) ([]*Message, error) {
    return []*Message{
        message.New(OrderShipped{...}, message.Attributes{
            Subject:     "order.shipped",
            Destination: "kafka://shipments",
        }),
    }, nil
}
```

### Adapter Responsibility

The adapter (e.g., `message/cloudevents`) maps between gopipe concepts and broker specifics:

```
┌─────────────────────────────────────────────────────────┐
│                        Engine                            │
│                                                          │
│  Subscriber.Subscribe(ctx, "orders")                     │
│       ↓                                                  │
│  [messages with Destination attribute]                   │
│       ↓                                                  │
│  Publisher.Publish(ctx, msgs)                            │
│       ↓                                                  │
│  Adapter extracts topic from Destination or Subject      │
└─────────────────────────────────────────────────────────┘
```

**Adapter extracts topic from:**
1. `Destination` attribute: `"kafka://shipments"` → topic `"shipments"`
2. `Subject` attribute: `"order.shipped"` → topic `"order.shipped"`
3. Default configured topic (fallback)

## CloudEvents Mapping

| gopipe | CloudEvents | Notes |
|--------|-------------|-------|
| `Subject` | `subject` | Optional, describes topic/category |
| `Destination` | N/A | gopipe extension for routing |
| `Source` | `source` | Required, origin URI |
| `Type` | `type` | Required, event type |

## Engine Routing (Future)

Engine routes to publishers by destination prefix:

```go
engine.AddPublisher("kafka://", kafkaPublisher)
engine.AddPublisher("nats://", natsPublisher)
engine.AddPublisher("http://", httpPublisher)

// Handler output routed by destination
return []*Message{
    message.New(event, message.Attributes{Destination: "kafka://orders"}),
    message.New(event, message.Attributes{Destination: "http://webhook"}),
}
```

## Summary

| Component | Topic/Destination | When | Why |
|-----------|-------------------|------|-----|
| Subscriber | `topic` parameter | Subscribe time | Dynamic subscriptions |
| Publisher | `Destination` attribute | Message creation | Handler decides routing |
| Adapter | Extracts from message | Publish time | Maps to broker topic |
