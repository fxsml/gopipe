# Topics and Routing

## Concepts

| Concept | Description | Example |
|---------|-------------|---------|
| **Topic** | Logical channel within a messaging system | `"orders"`, `"payments.received"` |
| **Input** | Channel into engine (with optional name) | `InputConfig{Name: "orders"}` |
| **Output** | Channel from engine (with Match pattern) | `OutputConfig{Match: "order.%"}` |
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
ordersOut := engine.AddOutput(message.OutputConfig{Match: "order.%"})
defaultOut := engine.AddOutput(message.OutputConfig{Match: "%"})

// External publishing (Publish runs in goroutine internally)
publisher := ce.NewPublisher(client)
publisher.Publish(ctx, ordersOut)

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
ordersOut := engine.AddOutput(message.OutputConfig{Match: "order.%"})
// Handler returns order.created → matches "order.%" → goes to ordersOut
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

Egress routing uses SQL LIKE pattern matching on CE type:

```go
type OutputConfig struct {
    Name  string  // optional, for logging/metrics
    Match string  // required, uses SQL LIKE syntax (% = any, _ = single char)
}

// Match patterns:
// "%"             - catch-all (default output)
// "order.%"       - prefix match (order.created, order.shipped)
// "%.created"     - suffix match (order.created, user.created)
// "order.created" - exact match
```

### Input Filtering

Defense-in-depth filtering on inputs using `message/match` package:

```go
import "github.com/fxsml/gopipe/message/match"

type InputConfig struct {
    Name    string   // optional, for tracing/metrics
    Matcher Matcher  // optional, filter incoming messages
}

// Filter by source and type
engine.AddInput(ch, message.InputConfig{
    Name: "order-events",
    Matcher: match.All(
        match.Sources("https://my-domain.com/orders/%"),
        match.Types("order.%"),
    ),
})

// Available matchers:
// match.All(...)      - AND combinator
// match.Any(...)      - OR combinator
// match.Sources(...)  - CE source patterns
// match.Types(...)    - CE type patterns
```

**Note:** Source filtering is defense-in-depth. The `source` attribute is self-declared by senders. True authentication requires transport-level security (mTLS, API keys).

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
loopback := engine.AddOutput(message.OutputConfig{Match: "validate.%"})

// Feed it back to engine as input
engine.AddInput(loopback, message.InputConfig{Name: "loopback"})
```

## Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                         Engine                                   │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──┐                             │
│     + Matcher          ↑           ├──> route ──> handler ──┐   │
│                  loopback ─────────┘                        │   │
│                     ↑                                       ↓   │
│                     │                                   marshal │
│                     │                                       │   │
│                     │          ┌─── Match: "order.%" ───────┤   │
│  AddOutput(cfg) ────┼──────────┼─── Match: "payment.%" ─────┤   │
│    returns          │          └─── Match: "%" ─────────────┘   │
│                     │                                           │
│              (no marshal for loopback - already typed)          │
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
| Engine | input | AddInput(ch, cfg) | `InputConfig{Name: "...", Matcher: ...}` |
| Handler | CE type | EventType() | `"order.created"` |
| Engine | output | AddOutput(cfg) | `OutputConfig{Match: "order.%"}` |
| Output routing | Match pattern | SQL LIKE on CE type | `"order.%"` matches `order.created` |
| Publisher | topic | Publish config | `"prod.shipments.v1"` |
