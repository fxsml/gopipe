# Topics and Routing

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Input** | Named channel into engine | `"orders"`, `"payments"` |
| **Output** | Named channel from engine | `"shipments"`, `"notifications"` |
| **CE Type** | CloudEvents type attribute | `"order.created"`, `"user.updated"` |

## Design

### Engine Doesn't Own I/O Lifecycle

Engine accepts and provides channels. External code manages subscription/publishing:

```go
// External subscription management
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput("orders", ch)

// External publishing management
publisher := ce.NewPublisher(client)
go publisher.Publish(ctx, engine.Output("shipments"))

// Start engine
done, _ := engine.Start(ctx)
```

**Why this separation?**
- Leader election: subscribe only when leader
- Dynamic scaling: add/remove inputs at runtime
- Multi-tenant: subscribe to tenant-specific topics

### Routing

**Two routing methods:**

```go
// CE type → handler
engine.RouteType("order.created", "process-orders")

// Handler → output (or handler for loopback)
engine.RouteOutput("process-orders", "shipments")
```

**Two routing strategies:**

| Strategy | Description |
|----------|-------------|
| `ConventionRouting` (default) | Auto-route by NamingStrategy |
| `ExplicitRouting` | Manual RouteType/RouteOutput required |

### NamingStrategy

Convention-based routing uses NamingStrategy:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
    OutputName(t reflect.Type) string  // Go type → output name
}

// KebabNaming (default):
// OrderCreated → CE type "order.created"
// OrderCreated → output "orders"
```

### Handler Types

**NewHandler - Explicit:**

Handler creates complete messages with all attributes:

```go
handler := message.NewHandler(func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
    return []*Message{
        message.New(OrderShipped{...}, message.Attributes{
            ID:          uuid.New().String(),
            SpecVersion: "1.0",
            Type:        "order.shipped",
            Source:      "/orders-service",
            Time:        time.Now(),
        }),
    }, nil
})
```

**NewCommandHandler - Convention:**

Handler returns typed events, CommandHandler generates CE attributes:

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, msg *TypedMessage[CreateOrder]) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        // Naming: message.KebabNaming (default)
    },
)
```

## Attribute Ownership

| Attribute | Owner |
|-----------|-------|
| `DataContentType` | Engine (from marshaler) |
| `Type` | Handler or NamingStrategy |
| `Source` | Handler or CommandHandlerConfig |
| `Subject` | Handler (explicit) |
| `ID`, `Time`, `SpecVersion` | Handler or CommandHandler |
| `CorrelationID` | Middleware |

## Loopback

Route handler output back to another handler:

```go
// With explicit routing:
engine.RouteOutput("validate-order", "process-order")  // handler → handler
```

## Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Engine                                    │
│                                                                  │
│  AddInput("orders", ch)                                          │
│       ↓                                                          │
│  unmarshal → route → handler → marshal                           │
│       ↓                                                          │
│  Output("shipments")                                             │
└─────────────────────────────────────────────────────────────────┘
         ↑                                       ↓
   Subscriber                              Publisher
   (external)                              (external)
```

**Adapter configuration (not in message):**
- Broker URL: `kafka://broker:9092`
- Topic mapping: output `"shipments"` → topic `"prod.shipments.v1"`
- Authentication, TLS, etc.

## Summary

| Component | Concept | When | Example |
|-----------|---------|------|---------|
| Subscriber | topic | Subscribe time | `"orders"` |
| Engine | input name | AddInput | `"orders"` |
| Handler | CE type | EventType() | `"order.created"` |
| Handler | output routing | NamingStrategy | `"orders"` |
| Engine | output name | Output() | `"shipments"` |
| Publisher | topic | Publish config | `"prod.shipments.v1"` |
