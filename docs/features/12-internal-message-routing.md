# Feature: Internal Message Routing via Topic

**Package:** `message`
**Status:** Proposed
**Related ADRs:**
- [ADR 0022](../adr/0022-internal-message-routing.md) - Internal Message Routing via Topic

## Summary

Topic-based internal message routing for composable pipelines without external broker dependency. Messages flow between handlers using the `topic` attribute for routing decisions.

## Motivation

- Composable internal pipelines without Kafka/Redis/etc.
- Clear routing using `topic` (destination) vs `type` (classification)
- Support for fan-out/fan-in patterns
- Easy integration with external systems at boundaries

## Implementation

### Internal Router

```go
// InternalRouter routes messages between handlers
type InternalRouter struct {
    routes      map[string][]Handler // topic -> handlers
    typeRoutes  map[string][]Handler // type -> handlers (fallback)
    serializer  *Serializer
}

func NewInternalRouter() *InternalRouter {
    return &InternalRouter{
        routes:     make(map[string][]Handler),
        typeRoutes: make(map[string][]Handler),
    }
}

// Route registers handler for topic
func (r *InternalRouter) Route(topic string, handler Handler) {
    r.routes[topic] = append(r.routes[topic], handler)
}

// RouteType registers handler for event type
func (r *InternalRouter) RouteType(eventType string, handler Handler) {
    r.typeRoutes[eventType] = append(r.typeRoutes[eventType], handler)
}

// RouteFunc convenience for inline handlers
func (r *InternalRouter) RouteFunc(topic string, fn func(context.Context, *Message) ([]*Message, error)) {
    r.Route(topic, HandlerFunc(fn))
}

// Handle routes message to appropriate handlers
func (r *InternalRouter) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    // Get topic from message
    topic, _ := msg.Attributes[AttrTopic].(string)

    // Find handlers for topic
    handlers := r.routes[topic]
    if len(handlers) == 0 {
        // Fallback to type-based routing
        eventType, _ := msg.Attributes[AttrType].(string)
        handlers = r.typeRoutes[eventType]
    }

    if len(handlers) == 0 {
        return nil, fmt.Errorf("no handler for topic=%s type=%s",
            topic, msg.Attributes[AttrType])
    }

    // Execute all handlers (fan-out)
    var allOutputs []*Message
    for _, h := range handlers {
        outputs, err := h.Handle(ctx, msg)
        if err != nil {
            return nil, err
        }
        allOutputs = append(allOutputs, outputs...)
    }

    return allOutputs, nil
}
```

### Internal Broker

```go
// InternalBroker provides in-process pub/sub
type InternalBroker struct {
    router     *InternalRouter
    input      chan *Message
    bufferSize int
    wg         sync.WaitGroup
}

func NewInternalBroker(router *InternalRouter, opts ...InternalBrokerOption) *InternalBroker {
    b := &InternalBroker{
        router:     router,
        bufferSize: 100,
    }
    for _, opt := range opts {
        opt(b)
    }
    b.input = make(chan *Message, b.bufferSize)
    return b
}

// Publish sends message to internal router
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
                // Log error, continue processing
                continue
            }
            // Route outputs back through broker
            for _, out := range outputs {
                select {
                case b.input <- out:
                case <-ctx.Done():
                    return ctx.Err()
                }
            }
            // Ack processed message
            msg.Ack()
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// Close stops the broker
func (b *InternalBroker) Close() error {
    close(b.input)
    return nil
}
```

### Pipeline Builder

```go
// PipelineBuilder creates composable internal pipelines
type PipelineBuilder struct {
    router *InternalRouter
}

func NewPipeline() *PipelineBuilder {
    return &PipelineBuilder{router: NewInternalRouter()}
}

// On registers handler for topic
func (p *PipelineBuilder) On(topic string, handler Handler) *PipelineBuilder {
    p.router.Route(topic, handler)
    return p
}

// OnFunc registers function handler for topic
func (p *PipelineBuilder) OnFunc(topic string, fn func(context.Context, *Message) ([]*Message, error)) *PipelineBuilder {
    p.router.RouteFunc(topic, fn)
    return p
}

// OnType registers handler for event type
func (p *PipelineBuilder) OnType(eventType string, handler Handler) *PipelineBuilder {
    p.router.RouteType(eventType, handler)
    return p
}

// Typed creates typed handler for topic
func (p *PipelineBuilder) Typed[In, Out any](
    topic string,
    handle func(context.Context, In) ([]Out, error),
    outputTopic string,
    outputType string,
) *PipelineBuilder {
    p.router.Route(topic, TypedHandler(handle,
        WithOutputTopic(outputTopic),
        WithOutputType(outputType),
    ))
    return p
}

// Build creates the internal broker
func (p *PipelineBuilder) Build(opts ...InternalBrokerOption) *InternalBroker {
    return NewInternalBroker(p.router, opts...)
}
```

### External Integration

```go
// Ingress: External -> Internal
func NewIngress(receiver Receiver, broker *InternalBroker) *Ingress {
    return &Ingress{receiver: receiver, broker: broker}
}

func (i *Ingress) Start(ctx context.Context) error {
    msgs, err := i.receiver.Receive(ctx)
    if err != nil {
        return err
    }
    for msg := range msgs {
        if err := i.broker.Publish(ctx, msg); err != nil {
            msg.Nack(err)
            continue
        }
    }
    return nil
}

// Egress: Internal -> External
func NewEgressHandler(sender Sender) Handler {
    return HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
        if err := sender.Send(ctx, []*Message{msg}); err != nil {
            return nil, err
        }
        return nil, nil // Terminal handler
    })
}
```

## Usage Example

### Simple Pipeline

```go
// Define handlers
handleOrder := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    order := msg.Data.(Order)

    return []*message.Message{
        message.MustNew(ShippingCommand{OrderID: order.ID}, message.Attributes{
            message.AttrTopic:       "shipping",
            message.AttrType:        "shipping.requested",
            message.AttrID:          uuid.NewString(),
            message.AttrSource:      "/orders",
            message.AttrSpecVersion: "1.0",
        }),
    }, nil
}

handleShipping := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    cmd := msg.Data.(ShippingCommand)
    // Process shipping...
    return []*message.Message{
        message.MustNew(NotificationEvent{OrderID: cmd.OrderID, Message: "Shipped!"}, message.Attributes{
            message.AttrTopic:       "notifications",
            message.AttrType:        "order.shipped",
            message.AttrID:          uuid.NewString(),
            message.AttrSource:      "/shipping",
            message.AttrSpecVersion: "1.0",
        }),
    }, nil
}

// Build pipeline
pipeline := message.NewPipeline().
    OnFunc("orders", handleOrder).
    OnFunc("shipping", handleShipping).
    OnFunc("notifications", handleNotifications).
    Build()

// Start
ctx := context.Background()
go pipeline.Start(ctx)

// Inject message
order := Order{ID: "123", Amount: 100}
msg := message.MustNew(order, message.Attributes{
    message.AttrTopic:       "orders",
    message.AttrType:        "order.created",
    message.AttrID:          uuid.NewString(),
    message.AttrSource:      "/api",
    message.AttrSpecVersion: "1.0",
})
pipeline.Publish(ctx, msg)
```

### Fan-Out Pattern

```go
// One topic, multiple handlers
pipeline := message.NewPipeline().
    OnFunc("orders", sendToShipping).
    OnFunc("orders", sendToInventory).
    OnFunc("orders", sendToAnalytics).
    Build()

// All three handlers receive every order
```

### With External Integration

```go
// HTTP receiver for external events
httpReceiver := broker.NewHTTPReceiver()

// Internal pipeline
pipeline := message.NewPipeline().
    OnFunc("orders", handleOrder).
    OnFunc("shipping", handleShipping).
    On("external.out", message.NewEgressHandler(httpSender)).
    Build()

// Start ingress
ingress := message.NewIngress(httpReceiver, pipeline)
go ingress.Start(ctx)

// Start pipeline
go pipeline.Start(ctx)
```

## Topic Naming Convention

```
<domain>.<entity>[.<action>]

Examples:
orders                   - All order events
orders.created          - Specific event
shipping.commands       - Commands for shipping domain
inventory.events        - Events from inventory
notifications.email     - Email notifications
external.out            - Egress to external systems
```

## Files Changed

- `message/internal_router.go` - InternalRouter implementation
- `message/internal_broker.go` - InternalBroker implementation
- `message/pipeline_builder.go` - PipelineBuilder for composition
- `message/ingress.go` - External -> Internal adapter
- `message/egress.go` - Internal -> External adapter
- `message/internal_router_test.go` - Router tests
- `message/pipeline_test.go` - Pipeline integration tests

## Testing Strategy

```go
func TestInternalRouter_FanOut(t *testing.T) {
    var calls []string
    router := NewInternalRouter()
    router.RouteFunc("orders", func(ctx context.Context, msg *Message) ([]*Message, error) {
        calls = append(calls, "handler1")
        return nil, nil
    })
    router.RouteFunc("orders", func(ctx context.Context, msg *Message) ([]*Message, error) {
        calls = append(calls, "handler2")
        return nil, nil
    })

    msg := MustNew("test", Attributes{
        AttrTopic: "orders",
        // ...other required attrs
    })

    _, err := router.Handle(context.Background(), msg)
    require.NoError(t, err)
    assert.Equal(t, []string{"handler1", "handler2"}, calls)
}
```

## Related Features

- [11-contenttype-serialization](11-contenttype-serialization.md) - Prerequisite for boundary serialization
- [04-message-router](04-message-router.md) - External router (complements this)
