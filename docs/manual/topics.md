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

**Ingress (CE type → handler) - automatic via NamingStrategy:**

```go
engine.AddHandler("process-orders", handler)
// Auto-derives: handler.EventType() → OrderCreated → "order.created"
// Registers: typeRoutes["order.created"] = "process-orders"
```

**Egress (handler output → output channel):**

```go
// Pattern matching in OutputConfig - first matching output wins
ordersOut := engine.AddOutput(message.OutputConfig{Matcher: match.Types("order.%")})
defaultOut := engine.AddOutput(message.OutputConfig{})  // nil = catch-all
// Handler returns order.created → matches "order.%" → goes to ordersOut
```

### NamingStrategy

NamingStrategy derives CE type from Go types. Used in two places:

1. **Engine**: Auto-derive ingress routing from `handler.EventType()`
2. **CommandHandler**: Auto-derive output CE type from event type `E`

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

// KebabNaming (default):
// OrderCreated → CE type "order.created"
```

### Matcher Interface

Both InputConfig and OutputConfig use the same Matcher interface:

```go
import "github.com/fxsml/gopipe/message/match"

type InputConfig struct {
    Name    string   // optional, for tracing/metrics
    Matcher Matcher  // optional, nil = match all
}

type OutputConfig struct {
    Name    string   // optional, for logging/metrics
    Matcher Matcher  // optional, nil = match all (catch-all)
}
```

**Default behavior:** Nil Matcher returns true for all messages (no expression evaluation).

### Match Package

```go
// Combinators
match.All(matchers...)   // AND - all must match
match.Any(matchers...)   // OR - at least one must match

// Attribute matchers (SQL LIKE syntax: % = any, _ = single char)
match.Sources(patterns...)  // match CE source
match.Types(patterns...)    // match CE type
```

### Output Routing

```go
// Register outputs - first matching wins
ordersOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("order.%"),
})
paymentsOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("payment.%"),
})
defaultOut := engine.AddOutput(message.OutputConfig{})  // nil = catch-all (register last)
```

### Input Filtering

Defense-in-depth filtering on inputs:

```go
engine.AddInput(ch, message.InputConfig{
    Name: "order-events",
    Matcher: match.All(
        match.Sources("https://my-domain.com/orders/%"),
        match.Types("order.%"),
    ),
})

// Accept all (default)
engine.AddInput(ch, message.InputConfig{})
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

Route output back for internal re-processing (bypasses marshal/unmarshal):

```go
engine.AddLoopback(message.LoopbackConfig{
    Name:    "validation-loop",
    Matcher: match.Types("validate.%"),
})
```

## Adapter Responsibility

The adapter (e.g., `message/cloudevents`) handles broker-specific details:

```
┌─────────────────────────────────────────────────────────────────┐
│                         Engine                                   │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──┐                             │
│     + Matcher                      ├──> route ──> handler ──┐   │
│             AddLoopback ───────────┘  (auto via Naming)     │   │
│                  ↑                                          ↓   │
│                  │                                      marshal │
│                  │                                          │   │
│                  │          ┌─── Matcher: "order.%" ────────┤   │
│  AddOutput(cfg) ─┼──────────┼─── Matcher: "payment.%" ──────┤   │
│    returns       │          └─── Matcher: nil (catch-all) ──┘   │
│                  │                                              │
│            (no marshal for loopback - already typed)            │
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
| Engine | routing | AddHandler (auto via NamingStrategy) | `OrderCreated` → `"order.created"` → handler |
| Handler | CE type | EventType() | `"order.created"` |
| Engine | output | AddOutput(cfg) | `OutputConfig{Matcher: match.Types("order.%")}` |
| Engine | loopback | AddLoopback(cfg) | `LoopbackConfig{Matcher: match.Types("validate.%")}` |
| Output routing | Match pattern | SQL LIKE on CE type | `"order.%"` matches `order.created` |
| Publisher | topic | Publish config | `"prod.shipments.v1"` |
