# Topics and Routing

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Input** | Channel into engine (with optional name) | `InputConfig{Name: "orders"}` |
| **Output** | Channel from engine (with Match pattern) | `OutputConfig{Match: "Order*"}` |
| **CE Type** | CloudEvents type attribute | `"order.created"`, `"user.updated"` |

## Design

### Engine Doesn't Own I/O Lifecycle

Engine accepts and provides channels. External code manages subscription/publishing:

```go
// Input: external subscription, optional name for tracing
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput(ch, message.InputConfig{Name: "order-events"})

// Output: pattern-based routing, returns channel directly
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
defaultOut := engine.AddOutput(message.OutputConfig{Match: "*"})

// External publishing
publisher := ce.NewPublisher(client)
go publisher.Publish(ctx, ordersOut)

// Start engine
done, _ := engine.Start(ctx)
```

**Why this separation?**
- Leader election: subscribe only when leader
- Dynamic scaling: add/remove inputs at runtime
- Multi-tenant: subscribe to tenant-specific topics

### Routing

**Ingress (CE type → handler):**

```go
// Explicit
engine.RouteType("order.created", "process-orders")

// Or convention: Handler's EventType() derives CE type via NamingStrategy
```

**Egress (handler output → output channel):**

```go
// Pattern matching in OutputConfig - no explicit routing needed
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
// Handler returns OrderCreated → matches "Order*" → goes to ordersOut
```

**Routing strategies (for ingress only):**

| Strategy | Description |
|----------|-------------|
| `ConventionRouting` (default) | Auto-route ingress by NamingStrategy |
| `ExplicitRouting` | Manual RouteType calls required |

### NamingStrategy

Convention-based ingress routing uses NamingStrategy:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
}

// KebabNaming (default):
// OrderCreated → CE type "order.created"
```

### Output Pattern Matching

Egress routing uses pattern matching on CE type:

```go
type OutputConfig struct {
    Name  string  // optional, for logging/metrics
    Match string  // required: "*", "Order*", "order.*", or CESQL
}

// Match patterns:
// "*"           - catch-all (default output)
// "Order*"      - Go type prefix match
// "order.*"     - CE type prefix match
// CESQL         - "type LIKE 'order.%' AND data.priority = 'high'"
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

Route output back to input for re-processing:

```go
// Create output for validation results
loopback := engine.AddOutput(message.OutputConfig{Match: "Validate*"})

// Feed it back to engine as input
engine.AddInput(loopback, message.InputConfig{Name: "loopback"})
```

## Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Engine                                    │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──> route ──> handler ──> marshal
│                                                              │   │
│                            ┌─── Match: "Order*" ─────────────┤   │
│  AddOutput(cfg) returns ───┼─── Match: "Payment*" ───────────┤   │
│                            └─── Match: "*" ──────────────────┘   │
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
| Engine | input | AddInput(ch, cfg) | `InputConfig{Name: "order-events"}` |
| Handler | CE type | EventType() | `"order.created"` |
| Engine | output | AddOutput(cfg) | `OutputConfig{Match: "Order*"}` |
| Output routing | Match pattern | Pattern match on CE type | `"Order*"` matches `OrderCreated` |
| Publisher | topic | Publish config | `"prod.shipments.v1"` |
