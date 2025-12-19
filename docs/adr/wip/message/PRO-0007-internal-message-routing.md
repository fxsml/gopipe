# ADR 0022: Internal Message Routing via Topic

**Date:** 2025-12-13
**Status:** Proposed
**Depends on:** ADR 0019, ADR 0020, ADR 0021

## Context

gopipe currently supports external message routing through brokers (HTTP, Kafka, etc.), but internal message routing between handlers requires awkward workarounds:

### Current Approach with NewProcessPipe

```go
// Works but verbose and type-heavy
pipe1 := gopipe.NewProcessPipe[*Message, *Message](handleOrders)
pipe2 := gopipe.NewProcessPipe[*Message, *Message](handleShipping)

// Manual channel wiring
out1 := pipe1.Start(ctx, in)
out2 := pipe2.Start(ctx, out1)
```

### Problems

1. **No Routing Logic**: Must manually wire channels based on message type
2. **Flat Structure**: Can't express "order.created goes to shipping AND inventory"
3. **External Dependency**: For routing, often need a message broker
4. **Type Gymnastics**: Generic types make composition verbose

### Goal

Enable composable internal pipelines without external systems, using `topic` for routing:

```go
// Desired: declarative internal routing
router := message.NewInternalRouter()
router.Route("orders", handleOrders)
router.Route("shipping", handleShipping)
router.Route("inventory", handleInventory)

// Messages flow internally based on topic
// No external broker needed
```

## Decision

Implement Topic-based internal message routing:

### 1. Topic as Routing Key

The `topic` attribute serves as the primary routing key for internal routing:

```go
// gopipe extension attribute
const AttrTopic = "topic"

// Message with topic for routing
msg := message.MustNew(order, message.Attributes{
    message.AttrTopic: "shipping.commands",  // Routing destination
    message.AttrType:  "shipping.requested", // Event type
    // ...other CE attributes
})
```

### 2. Topic vs Type

| Attribute | Purpose | Example |
|-----------|---------|---------|
| `topic` | **Routing destination** (where to send) | `"orders"`, `"shipping.commands"` |
| `type` | **Event classification** (what it is) | `"order.created"`, `"payment.received"` |

A single topic can receive multiple event types:
```
Topic: "orders"
  - order.created
  - order.updated
  - order.cancelled

Topic: "shipping.commands"
  - shipping.requested
  - shipping.cancelled
```

### 3. InternalRouter

```go
// InternalRouter routes messages between handlers without external systems
type InternalRouter struct {
    routes      map[string][]Handler      // topic -> handlers
    typeRoutes  map[string][]Handler      // type -> handlers (fallback)
    defaultTopic string                   // Default topic if not specified
    serializer  *Serializer               // For type conversion if needed
}

// Route registers a handler for a topic
func (r *InternalRouter) Route(topic string, handler Handler) {
    r.routes[topic] = append(r.routes[topic], handler)
}

// RouteType registers a handler for a specific event type
func (r *InternalRouter) RouteType(eventType string, handler Handler) {
    r.typeRoutes[eventType] = append(r.typeRoutes[eventType], handler)
}

// RouteFunc is a convenience for inline handlers
func (r *InternalRouter) RouteFunc(topic string, fn func(context.Context, *Message) ([]*Message, error)) {
    r.Route(topic, HandlerFunc(fn))
}
```

### 4. Internal Pub/Sub

```go
// InternalBroker provides in-process pub/sub
type InternalBroker struct {
    router     *InternalRouter
    input      chan *Message
    bufferSize int
}

// NewInternalBroker creates an internal message broker
func NewInternalBroker(router *InternalRouter, opts ...InternalBrokerOption) *InternalBroker {
    return &InternalBroker{
        router:     router,
        input:      make(chan *Message, 100),
        bufferSize: 100,
    }
}

// Publish sends a message to the internal router
func (b *InternalBroker) Publish(ctx context.Context, msg *Message) error {
    select {
    case b.input <- msg:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

// Start begins processing messages
func (b *InternalBroker) Start(ctx context.Context) error {
    for {
        select {
        case msg := <-b.input:
            outputs, err := b.router.Handle(ctx, msg)
            if err != nil {
                // Handle error (log, nack, etc.)
                continue
            }
            // Route output messages
            for _, out := range outputs {
                b.input <- out
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### 5. Composable Pipeline Builder

```go
// PipelineBuilder creates composable internal pipelines
type PipelineBuilder struct {
    router *InternalRouter
}

// NewPipeline creates a pipeline builder
func NewPipeline() *PipelineBuilder {
    return &PipelineBuilder{
        router: NewInternalRouter(),
    }
}

// On registers a handler for topic
func (p *PipelineBuilder) On(topic string, handler Handler) *PipelineBuilder {
    p.router.Route(topic, handler)
    return p
}

// OnType registers a handler for event type
func (p *PipelineBuilder) OnType(eventType string, handler Handler) *PipelineBuilder {
    p.router.RouteType(eventType, handler)
    return p
}

// Typed creates a typed handler wrapper
func (p *PipelineBuilder) Typed[In, Out any](
    topic string,
    handle func(context.Context, In) ([]Out, error),
    outputTopic string,
) *PipelineBuilder {
    p.router.Route(topic, TypedHandler(handle, WithOutputTopic(outputTopic)))
    return p
}

// Build creates the internal broker
func (p *PipelineBuilder) Build() *InternalBroker {
    return NewInternalBroker(p.router)
}
```

### 6. Usage Example: Order Processing Pipeline

```go
// Define handlers
handleOrder := func(ctx context.Context, msg *Message) ([]*Message, error) {
    order := msg.Data.(Order)

    // Validate order...

    // Emit to shipping and inventory topics
    return []*Message{
        message.MustNew(ShippingCommand{OrderID: order.ID}, message.Attributes{
            message.AttrTopic: "shipping",
            message.AttrType:  "shipping.requested",
            // ...
        }),
        message.MustNew(InventoryReservation{OrderID: order.ID}, message.Attributes{
            message.AttrTopic: "inventory",
            message.AttrType:  "inventory.reserve",
            // ...
        }),
    }, nil
}

handleShipping := func(ctx context.Context, msg *Message) ([]*Message, error) {
    cmd := msg.Data.(ShippingCommand)
    // Process shipping...
    return []*Message{
        message.MustNew(ShippingEvent{OrderID: cmd.OrderID, Status: "shipped"}, message.Attributes{
            message.AttrTopic: "notifications",
            message.AttrType:  "order.shipped",
            // ...
        }),
    }, nil
}

// Build pipeline
pipeline := message.NewPipeline().
    On("orders", message.HandlerFunc(handleOrder)).
    On("shipping", message.HandlerFunc(handleShipping)).
    On("inventory", message.HandlerFunc(handleInventory)).
    On("notifications", message.HandlerFunc(handleNotifications)).
    Build()

// Start processing
go pipeline.Start(ctx)

// Inject messages
pipeline.Publish(ctx, orderMessage)
```

### 7. Fan-Out and Fan-In

```go
// Fan-out: one topic to multiple handlers
router.Route("orders", handlerA)
router.Route("orders", handlerB)
router.Route("orders", handlerC)
// All three handlers receive every message on "orders"

// Fan-in: multiple topics to one handler
router.Route("payment.success", paymentHandler)
router.Route("payment.failed", paymentHandler)
// paymentHandler receives from both topics
```

### 8. External System Integration

Connect internal routing to external systems at boundaries:

```go
// External -> Internal (ingress)
externalReceiver := broker.NewHTTPReceiver(...)
go func() {
    msgs, _ := externalReceiver.Receive(ctx)
    for msg := range msgs {
        // Route externally received messages internally
        pipeline.Publish(ctx, msg)
    }
}()

// Internal -> External (egress)
router.Route("external.notifications", func(ctx context.Context, msg *Message) ([]*Message, error) {
    // Send to external system
    externalSender.Send(ctx, []*Message{msg})
    return nil, nil  // Terminal handler
})
```

### 9. Topic Naming Convention

Recommended topic naming:

```
<domain>.<entity>.<action>

Examples:
- orders.created
- orders.commands.create
- shipping.events
- inventory.commands.reserve
- notifications.email
```

## Rationale

1. **No External Dependency**: Internal messaging without Kafka/Redis/etc.
2. **Topic for Routing**: Clear separation from `type` (classification)
3. **Composability**: Pipeline builder enables declarative composition
4. **CloudEvents Aligned**: Topic is an extension, not violating spec
5. **Fan-Out/Fan-In**: Supports complex routing patterns
6. **Boundary Integration**: Easy to connect to external systems

## Consequences

### Positive

- True internal composable pipelines
- No broker dependency for simple workflows
- Clear routing based on topic
- Works seamlessly with external systems at boundaries
- Supports saga and workflow patterns

### Negative

- In-memory only (no persistence without external broker)
- Must explicitly set topic for routing
- Potential confusion between topic and type
- No guaranteed delivery without external broker

### Trade-offs

| Feature | Internal Routing | External Broker |
|---------|-----------------|-----------------|
| Latency | Microseconds | Milliseconds |
| Persistence | None | Yes |
| Scaling | Single process | Distributed |
| Complexity | Simple | Higher |
| Use Case | In-process workflows | Distributed systems |

## Links

- [ADR 0019: CloudEvents Mandatory](0019-cloudevents-mandatory.md)
- [ADR 0020: Non-Generic Message](0020-non-generic-message.md)
- [ADR 0021: ContentType Serialization](0021-contenttype-serialization.md)
- [Feature 12: Internal Message Routing](../features/12-internal-message-routing.md)
- [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md)
- [CloudEvents Primer - Routing](https://github.com/cloudevents/spec/blob/main/cloudevents/primer.md)
