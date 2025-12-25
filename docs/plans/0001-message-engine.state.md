# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-25

## Current Design Decisions

### Engine Orchestrates, Doesn't Own I/O Lifecycle

Engine processes channels, doesn't manage subscription/publishing lifecycle.

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

### Routing

**Two explicit methods:**
```go
engine.RouteType("order.created", "process-orders")   // CE type → handler
engine.RouteOutput("process-orders", "shipments")     // handler → output (or handler for loopback)
```

**Two strategies:**
- `ConventionRouting` (default): Auto-route by naming conventions
- `ExplicitRouting`: Manual Route* calls required

**Convention rules:**
- Ingress: `EventType()` always explicit (Handler interface)
- Egress: `OrderCreated` → `orders` output (domain prefix, pluralized)

### Handler Interface

```go
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, event any) ([]*Message, error)
}

// Name is registration concern, not handler property
engine.AddHandler("process-orders", handler)
```

### Attributes

- `Subscriber`: Set by engine on ingress (input channel name)
- `Publisher`: Optional per-message override for output routing

### Publisher/Topic Separation

- Engine routes to **outputs by name** (domain-level: `orders`, `payments`)
- Publisher adapter handles **topic mapping** (broker-level concern)

```go
publisher := kafka.NewPublisher(client, kafka.Config{
    Topic: "orders",  // or TopicFunc for dynamic
})
```

## Rejected Ideas

### Handler owns its name
```go
// Rejected: Name() in Handler interface
type Handler interface {
    Name() string  // ❌ Name is wiring concern, not handler logic
    EventType() reflect.Type
    Handle(...)
}
```
**Why rejected:** Name is a registration/wiring concern, not intrinsic to handler logic.

### Destination/Publisher attribute for routing
```go
// Rejected: Handler sets destination in message
message.New(event, Attributes{Destination: "shipments"})  // ❌
```
**Why rejected:** Routing should be by type/convention, not per-message attribute. Attribute kept only as escape hatch.

### Source attribute for subscriber name
```go
// Rejected: Reuse CloudEvents Source
msg.Attributes[AttrSource] = subscriberName  // ❌
```
**Why rejected:** `Source` is a required CloudEvents attribute (origin URI). Can't repurpose.

### Engine owns Subscriber/Publisher lifecycle
```go
// Rejected: Engine manages subscription
engine.AddSubscriber("orders", subscriber)  // ❌ Engine subscribes internally
```
**Why rejected:** Doesn't handle leader election, dynamic scaling, multi-tenant patterns. Subscription lifecycle is external concern.

### Type-based publisher routing
```go
// Rejected: Route by output event type
engine.Route(OrderShipped{}, "shipments")  // ❌
```
**Why rejected:** Handler name is already explicit. Type-based adds complexity. Convention handles common case.

### Single Route() method for both directions
```go
// Rejected: Ambiguous what from/to are
engine.Route("order.created", "process-orders")  // ❌ Is this type→handler or handler→output?
```
**Why rejected:** Two methods (`RouteType`, `RouteOutput`) are more explicit.

## Open Questions

1. Should `Output()` return a channel or require registration first?
2. Loopback: Is `RouteOutput("handler-a", "handler-b")` enough or need explicit `RouteLoop()`?
3. Default output for unrouted handler output?

## Next Steps

1. Update Plan 0001 with this design
2. Update topics.md manual
3. Consider if ADR 0022 needs updates
