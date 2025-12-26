# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-26

## Current Design Decisions

### Separation of Concerns

| Component | Responsibility |
|-----------|----------------|
| **Engine** | Orchestrates flow, routing, lifecycle |
| **Marshaler** | Serialization + type registry |
| **Handler** | Business logic, creates output messages |
| **NamingStrategy** | Convention for deriving CE types/names |
| **Subscriber/Publisher** | I/O adapters (external lifecycle) |

### Engine - Orchestration Only

Engine accepts channels, doesn't own I/O lifecycle.

```go
// Input: Engine accepts named channels
engine.AddInput("order-events", ch)

// Output: Engine provides named output channels
out := engine.Output("shipments")

// External code manages subscription
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput("order-events", ch)

// External code manages publishing
publisher := ce.NewPublisher(client)
publisher.Publish(ctx, engine.Output("shipments"))
```

### Marshaler - Lightweight with Registry

Marshaler handles serialization and type registry (needed for unmarshal).

```go
type Marshaler interface {
    Register(v any)                                // register type, derive CE type name
    Marshal(v any) ([]byte, string, error)         // returns data, CE type, error
    Unmarshal(data []byte, ceType string) (any, error)
    ContentType() string
}
```

Registry is required because `Unmarshal` receives CE type string, needs to know which Go type to instantiate.

### Handler - Constructors, Not Engine Methods

Handlers created via constructors, registered by name on engine.

```go
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, event any) ([]*Message, error)
}

// Generic constructor
func NewHandler[T any](fn func(ctx context.Context, event T) ([]*Message, error)) Handler

// CQRS constructor - always returns slice of events
func NewCommandHandler[C, E any](fn func(ctx context.Context, cmd C) ([]E, error)) Handler

// With config (not variadic - single optional config)
func NewCommandHandlerWithConfig[C, E any](
    fn func(ctx context.Context, cmd C) ([]E, error),
    cfg CommandHandlerConfig,
) Handler

// Registration
engine.AddHandler("process-order", handler)
```

### Handler Creates Complete Messages

Handler is responsible for all message attributes (except auto-fields).

```go
handler := message.NewHandler(func(ctx context.Context, order OrderCreated) ([]*Message, error) {
    return []*Message{
        message.New(OrderShipped{OrderID: order.ID}, message.Attributes{
            Type:   "order.shipped",
            Source: "/orders-service",
        }),
    }, nil
})
```

Engine only adds auto-fields:
- `ID` (if not set) - auto-generate UUID
- `SpecVersion` - "1.0"
- `Time` - now()
- `Subscriber` - input channel name

### Routing - Two Explicit Methods

```go
engine.RouteType("order.created", "process-orders")   // CE type → handler
engine.RouteOutput("process-orders", "shipments")     // handler → output (or handler for loopback)
```

### Routing Strategies

```go
type RoutingStrategy int

const (
    ConventionRouting RoutingStrategy = iota  // default: auto-route by naming
    ExplicitRouting                           // manual Route* calls required
)
```

**Convention rules:**
- Ingress: Handler's `EventType()` derives CE type name
- Egress: `OrderCreated` → `orders` output (domain prefix, pluralized)

### NamingStrategy - Separate Concern

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
    OutputName(t reflect.Type) string  // Go type → output name
}

// CQRS convention
var CQRSNaming NamingStrategy  // CreateOrder→"create.order", OrderCreated→"orders"
```

### Validation - Optional Middleware

```go
engine.Use(message.ValidateCE())  // validates required CE attributes
```

### Attributes

- `Subscriber`: Set by engine on ingress (input channel name)
- No per-message routing attribute by default (convention-based)

## Rejected Ideas

### Handler owns its name
```go
type Handler interface {
    Name() string  // ❌
}
```
**Why rejected:** Name is wiring concern, not handler logic.

### Variadic config for handlers
```go
func NewCommandHandler[C, E any](fn ..., cfg ...Config) Handler  // ❌
```
**Why rejected:** Not idiomatic Go. Use separate `NewCommandHandlerWithConfig` function.

### Per-message routing attribute
```go
message.New(event, Attributes{Publisher: "shipments"})  // ❌ as default
```
**Why rejected:** Routing should be by type/convention. Attribute only as escape hatch.

### Engine owns I/O lifecycle
```go
engine.AddSubscriber("orders", subscriber)  // ❌ Engine subscribes internally
```
**Why rejected:** Doesn't handle leader election, dynamic scaling. External concern.

### Heavy Marshaler with naming
```go
type Marshaler interface {
    TypeName(v any) string     // ❌ separate concern
    IsCommand(v any) bool      // ❌ separate concern
}
```
**Why rejected:** Marshaler should be lightweight. Naming is separate strategy.

### Single ambiguous Route() method
```go
engine.Route("order.created", "process-orders")  // ❌
```
**Why rejected:** Two methods are more explicit.

## Idiomaticity Review

| Aspect | Pattern | Idiomatic? |
|--------|---------|------------|
| Handler constructors | `NewHandler[T]()`, `NewHandlerWithConfig[T]()` | ✅ Go generic pattern |
| Config pattern | Separate function, not variadic | ✅ Explicit over magic |
| Interface segregation | Marshaler, NamingStrategy, Handler separate | ✅ Single responsibility |
| Engine lifecycle | Accepts channels, doesn't own I/O | ✅ Composition over inheritance |
| Routing methods | `RouteType`, `RouteOutput` | ✅ Explicit naming |
| Optional features | Middleware (`Use()`) | ✅ Standard pattern |
| Default + override | ConventionRouting default, explicit override | ✅ Progressive disclosure |

## Open Questions

1. Should `Output()` return a channel or require registration first?
2. Loopback: Is `RouteOutput("handler-a", "handler-b")` enough?
3. Default output for unrouted handler output?
4. Should `NewCommandHandler` auto-register types with marshaler?

## Next Steps

1. Update Plan 0001 with refined design
2. Update ADR 0022 with marshaler/handler separation
3. Create NamingStrategy interface specification
