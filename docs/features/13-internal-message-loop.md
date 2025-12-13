# Feature: Internal Message Loop

**Package:** `message`
**Status:** Proposed
**Related ADRs:**
- [ADR 0023](../adr/0023-internal-message-loop.md) - Internal Message Loop
- [ADR 0024](../adr/0024-destination-attribute.md) - Destination Attribute

## Summary

A complete internal messaging system with feedback loop capability, enabling fully composable pipelines without external dependencies. Messages route internally via `gopipe://` destinations and can "break out" to external systems via other URI schemes.

## Motivation

- Full pipeline functionality without generics complexity
- Pluggable transport backends (Go channels, NATS)
- Clear internal/external boundary via destination URI
- Adapter pattern for existing Publisher/Subscriber interfaces

## Implementation

### MessageChannel Interface

```go
// MessageChannel combines pub/sub for internal message routing
type MessageChannel interface {
    Publish(ctx context.Context, msg *Message) error
    Subscribe(ctx context.Context, topics ...string) (<-chan *Message, error)
    Close() error
}
```

### NoopChannel (Go Channel Implementation)

```go
// NoopChannel implements MessageChannel using plain Go channels
type NoopChannel struct {
    mu          sync.RWMutex
    topics      map[string][]chan *Message
    bufferSize  int
    closed      bool
}

func NewNoopChannel(config MessageChannelConfig) *NoopChannel {
    bufSize := config.BufferSize
    if bufSize <= 0 {
        bufSize = 100
    }
    return &NoopChannel{
        topics:     make(map[string][]chan *Message),
        bufferSize: bufSize,
    }
}

func (c *NoopChannel) Publish(ctx context.Context, msg *Message) error {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if c.closed {
        return ErrChannelClosed
    }

    dest, err := msg.ParsedDestination()
    if err != nil || !dest.IsInternal() {
        return ErrNotInternalDestination
    }

    // Fan-out to all subscribers for this path
    for _, sub := range c.topics[dest.Path] {
        select {
        case sub <- msg:
        case <-ctx.Done():
            return ctx.Err()
        default:
            // Buffer full - backpressure
        }
    }
    return nil
}

func (c *NoopChannel) Subscribe(ctx context.Context, topics ...string) (<-chan *Message, error) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.closed {
        return nil, ErrChannelClosed
    }

    ch := make(chan *Message, c.bufferSize)
    for _, topic := range topics {
        c.topics[topic] = append(c.topics[topic], ch)
    }
    return ch, nil
}
```

### Publisher/Subscriber Adapters

```go
// ChannelPublisher adapts MessageChannel to Publisher
type ChannelPublisher struct {
    channel MessageChannel
}

func NewChannelPublisher(ch MessageChannel) *ChannelPublisher {
    return &ChannelPublisher{channel: ch}
}

func (p *ChannelPublisher) Publish(ctx context.Context, msgs ...*Message) error {
    for _, msg := range msgs {
        if err := p.channel.Publish(ctx, msg); err != nil {
            return err
        }
    }
    return nil
}

// ChannelSubscriber adapts MessageChannel to Subscriber
type ChannelSubscriber struct {
    channel MessageChannel
    topics  []string
}

func NewChannelSubscriber(ch MessageChannel, topics ...string) *ChannelSubscriber {
    return &ChannelSubscriber{channel: ch, topics: topics}
}

func (s *ChannelSubscriber) Subscribe(ctx context.Context) (<-chan *Message, error) {
    return s.channel.Subscribe(ctx, s.topics...)
}
```

### InternalLoop Component

```go
// InternalLoop provides complete internal messaging with routing
type InternalLoop struct {
    channel    MessageChannel
    router     *InternalRouter
    external   *ExternalDispatcher
    serializer *Serializer
    config     InternalLoopConfig
}

type InternalLoopConfig struct {
    BufferSize int
    MaxHops    int  // Loop detection
    Serializer *Serializer
}

func NewInternalLoop(opts ...InternalLoopOption) *InternalLoop {
    loop := &InternalLoop{
        channel: NewNoopChannel(MessageChannelConfig{BufferSize: 100}),
        router:  NewInternalRouter(),
        external: NewExternalDispatcher(),
        config:  InternalLoopConfig{MaxHops: 10},
    }
    for _, opt := range opts {
        opt(loop)
    }
    return loop
}

// Route registers handler for internal path
func (l *InternalLoop) Route(path string, handler Handler) {
    l.router.Route(path, handler)
}

// RegisterExternal registers external sender for scheme
func (l *InternalLoop) RegisterExternal(scheme string, sender Sender) {
    l.external.Register(scheme, sender)
}

// Start begins processing
func (l *InternalLoop) Start(ctx context.Context) error {
    topics := l.router.Topics()
    msgs, err := l.channel.Subscribe(ctx, topics...)
    if err != nil {
        return err
    }

    go func() {
        for msg := range msgs {
            // Loop detection
            hops := msg.IncrementHops()
            if hops > l.config.MaxHops {
                msg.Nack(ErrMaxHopsExceeded)
                continue
            }

            // Route through handlers
            outputs, err := l.router.Handle(ctx, msg)
            if err != nil {
                msg.Nack(err)
                continue
            }

            // Dispatch outputs
            for _, out := range outputs {
                if out.IsInternalDestination() {
                    l.channel.Publish(ctx, out)
                } else {
                    l.external.Dispatch(ctx, out)
                }
            }
            msg.Ack()
        }
    }()

    return nil
}

// Inject publishes message into the loop
func (l *InternalLoop) Inject(ctx context.Context, msg *Message) error {
    return l.channel.Publish(ctx, msg)
}
```

### External Dispatcher

```go
// ExternalDispatcher routes to external systems by scheme
type ExternalDispatcher struct {
    senders map[string]Sender
}

func NewExternalDispatcher() *ExternalDispatcher {
    return &ExternalDispatcher{senders: make(map[string]Sender)}
}

func (d *ExternalDispatcher) Register(scheme string, sender Sender) {
    d.senders[scheme] = sender
}

func (d *ExternalDispatcher) Dispatch(ctx context.Context, msg *Message) error {
    dest, err := msg.ParsedDestination()
    if err != nil {
        return err
    }

    sender, ok := d.senders[string(dest.Scheme)]
    if !ok {
        return fmt.Errorf("no sender for scheme: %s", dest.Scheme)
    }

    return sender.Send(ctx, []*Message{msg})
}
```

## Usage Example

### Basic Internal Pipeline

```go
// Create internal loop
loop := message.NewInternalLoop()

// Register handlers
loop.Route("orders", message.HandlerFunc(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    order := msg.Data.(Order)

    // Process and route to shipping (internal)
    return []*message.Message{
        message.MustNew(ShippingCommand{OrderID: order.ID}, message.Attributes{
            message.AttrDestination: "gopipe://shipping",
            message.AttrType:        "shipping.requested",
            // ...other CE attrs
        }),
    }, nil
}))

loop.Route("shipping", message.HandlerFunc(handleShipping))
loop.Route("notifications", message.HandlerFunc(handleNotifications))

// Start the loop
ctx := context.Background()
loop.Start(ctx)

// Inject messages
order := Order{ID: "123", Amount: 100}
msg := message.MustNew(order, message.Attributes{
    message.AttrDestination: "gopipe://orders",
    message.AttrType:        "order.created",
    message.AttrID:          uuid.NewString(),
    message.AttrSource:      "/api/orders",
    message.AttrSpecVersion: "1.0",
})
loop.Inject(ctx, msg)
```

### With External Break-Out

```go
// Create loop with external senders
loop := message.NewInternalLoop()

// Register external senders
loop.RegisterExternal("kafka", kafkaSender)
loop.RegisterExternal("http", httpSender)

// Handler that breaks out
loop.Route("complete", message.HandlerFunc(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    event := msg.Data.(OrderCompleteEvent)

    return []*message.Message{
        // Internal: audit log
        message.MustNew(AuditEvent{...}, message.Attributes{
            message.AttrDestination: "gopipe://audit",
            // ...
        }),
        // External: Kafka notification
        message.MustNew(NotificationEvent{...}, message.Attributes{
            message.AttrDestination: "kafka://notifications/order-complete",
            // ...
        }),
        // External: HTTP webhook
        message.MustNew(WebhookPayload{...}, message.Attributes{
            message.AttrDestination: "http://partner.example.com/webhook",
            // ...
        }),
    }, nil
}))
```

### Full Pipeline Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              gopipe InternalLoop                                 │
│                                                                                  │
│    ┌─────────────────────────────────────────────────────────────────────────┐  │
│    │                         NoopChannel (Go channels)                        │  │
│    │                                                                          │  │
│    │   gopipe://orders  gopipe://shipping  gopipe://audit  gopipe://complete │  │
│    │        │                  │                │                │           │  │
│    └────────┼──────────────────┼────────────────┼────────────────┼───────────┘  │
│             │                  │                │                │              │
│             ▼                  ▼                ▼                ▼              │
│    ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ │
│    │ OrderHandler   │ │ShippingHandler │ │  AuditHandler  │ │CompleteHandler │ │
│    │                │ │                │ │                │ │                │ │
│    │ order.created  │ │shipping.request│ │ audit.event    │ │order.complete  │ │
│    └───────┬────────┘ └───────┬────────┘ └───────┬────────┘ └───────┬────────┘ │
│            │                  │                  │                  │          │
│            ▼                  ▼                  ▼                  │          │
│      gopipe://           gopipe://          gopipe://               │          │
│      shipping            audit              (sink)                  │          │
│            │                  │                                     │          │
│            └──────────────────┘                                     │          │
│                    │                                                │          │
│                    │ (internal routing via NoopChannel)             │          │
│                    │                                                │          │
└────────────────────┼────────────────────────────────────────────────┼──────────┘
                     │                                                │
                     │                              ┌─────────────────┘
                     │                              │
                     │                              │ kafka://notifications/...
                     │                              │ http://partner.com/webhook
                     │                              ▼
                     │                     ┌────────────────────┐
                     │                     │ ExternalDispatcher │
                     │                     │                    │
                     │                     │ ┌────────────────┐ │
                     │                     │ │ KafkaSender    │─┼──> Kafka
                     │                     │ └────────────────┘ │
                     │                     │ ┌────────────────┐ │
                     │                     │ │ HTTPSender     │─┼──> HTTP
                     │                     │ └────────────────┘ │
                     │                     └────────────────────┘
                     │
        ┌────────────┴────────────┐
        │  External Input         │
        │  (HTTP Receiver, etc)   │
        └─────────────────────────┘
```

## Files Changed

- `message/channel.go` - MessageChannel interface
- `message/noop_channel.go` - NoopChannel implementation
- `message/channel_adapter.go` - Publisher/Subscriber adapters
- `message/internal_loop.go` - InternalLoop component
- `message/external_dispatcher.go` - External routing dispatcher
- `message/destination.go` - Destination parsing utilities
- `message/loop_test.go` - Internal loop tests

## Testing Strategy

```go
func TestInternalLoop_FullPipeline(t *testing.T) {
    var processed []string
    mu := sync.Mutex{}

    loop := NewInternalLoop()

    loop.Route("step1", HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
        mu.Lock()
        processed = append(processed, "step1")
        mu.Unlock()
        return []*Message{
            MustNew("data", Attributes{
                AttrDestination: "gopipe://step2",
                // ...CE attrs
            }),
        }, nil
    }))

    loop.Route("step2", HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
        mu.Lock()
        processed = append(processed, "step2")
        mu.Unlock()
        return nil, nil
    }))

    ctx := context.Background()
    loop.Start(ctx)

    // Inject initial message
    loop.Inject(ctx, MustNew("start", Attributes{
        AttrDestination: "gopipe://step1",
        // ...CE attrs
    }))

    // Wait for processing
    time.Sleep(100 * time.Millisecond)

    assert.Equal(t, []string{"step1", "step2"}, processed)
}
```

## Related Features

- [09-cloudevents-mandatory](09-cloudevents-mandatory.md) - Prerequisite
- [12-internal-message-routing](12-internal-message-routing.md) - Foundation
- [14-nats-integration](14-nats-integration.md) - Alternative channel implementation
