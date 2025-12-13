# ADR 0023: Internal Message Loop

**Date:** 2025-12-13
**Status:** Proposed
**Depends on:** ADR 0019, ADR 0020, ADR 0021, ADR 0022, ADR 0024

## Context

ADR 0022 introduced internal message routing via Topic. However, to enable true composable pipelines with full messaging capabilities, we need a **feedback loop** component that:

1. Combines subscriber and publisher into a single component
2. Routes messages internally without external system dependency
3. Allows "break-out" to external systems when needed
4. Supports any underlying transport (Go channel, NATS, etc.)

### Current Limitation

The current internal routing (ADR 0022) provides handler-to-handler routing but lacks:
- A unified abstraction for internal pub/sub
- Pluggable transport backends
- Clear boundary between internal and external messaging
- Ability to swap implementations (noop channel vs NATS)

### Goal

Provide a **Channel** abstraction that:
- Works like a Go channel but with messaging semantics
- Supports topic-based routing
- Is pluggable (noop, NATS, etc.)
- Integrates with existing Publisher/Subscriber interfaces

## Decision

Introduce an **Internal Message Loop** architecture with pluggable channel implementations.

### 1. MessageChannel Interface

```go
// MessageChannel combines pub/sub for internal message routing
type MessageChannel interface {
    // Publish sends a message to the channel
    Publish(ctx context.Context, msg *Message) error

    // Subscribe returns a channel of messages for the given topics
    Subscribe(ctx context.Context, topics ...string) (<-chan *Message, error)

    // Close shuts down the channel
    Close() error
}

// MessageChannelConfig configures the message channel
type MessageChannelConfig struct {
    BufferSize     int           // Channel buffer size
    Serializer     *Serializer   // Optional serializer for typed channels
    DefaultTimeout time.Duration // Default operation timeout
}
```

### 2. Noop Implementation (Go Channel)

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

    // Get destination from message
    dest := msg.Destination()
    if dest == "" {
        return ErrNoDestination
    }

    // Parse destination URI
    topic, err := parseInternalDestination(dest)
    if err != nil {
        return err // Not an internal destination
    }

    // Fan-out to all subscribers
    subscribers := c.topics[topic]
    for _, sub := range subscribers {
        select {
        case sub <- msg:
        case <-ctx.Done():
            return ctx.Err()
        default:
            // Buffer full - could log or handle backpressure
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

func (c *NoopChannel) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.closed = true
    for _, subs := range c.topics {
        for _, ch := range subs {
            close(ch)
        }
    }
    return nil
}
```

### 3. Publisher/Subscriber Adapters

```go
// ChannelPublisher adapts MessageChannel to Publisher interface
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

// ChannelSubscriber adapts MessageChannel to Subscriber interface
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

### 4. Internal Loop Component

```go
// InternalLoop provides a complete internal messaging system
type InternalLoop struct {
    channel    MessageChannel
    router     *InternalRouter
    serializer *Serializer
    running    bool
    wg         sync.WaitGroup
}

func NewInternalLoop(opts ...InternalLoopOption) *InternalLoop {
    loop := &InternalLoop{
        channel: NewNoopChannel(MessageChannelConfig{}),
        router:  NewInternalRouter(),
    }
    for _, opt := range opts {
        opt(loop)
    }
    return loop
}

// WithChannel sets a custom MessageChannel implementation
func WithChannel(ch MessageChannel) InternalLoopOption {
    return func(l *InternalLoop) { l.channel = ch }
}

// Route registers a handler for internal destination
func (l *InternalLoop) Route(path string, handler Handler) {
    l.router.Route(path, handler)
}

// Start begins processing the internal loop
func (l *InternalLoop) Start(ctx context.Context) error {
    // Subscribe to all registered routes
    topics := l.router.Topics()
    msgs, err := l.channel.Subscribe(ctx, topics...)
    if err != nil {
        return err
    }

    l.running = true
    l.wg.Add(1)
    go func() {
        defer l.wg.Done()
        for msg := range msgs {
            outputs, err := l.router.Handle(ctx, msg)
            if err != nil {
                msg.Nack(err)
                continue
            }

            // Route outputs back through loop or to external
            for _, out := range outputs {
                if isInternalDestination(out.Destination()) {
                    l.channel.Publish(ctx, out)
                } else {
                    // External destination - emit to external handler
                    l.emitExternal(ctx, out)
                }
            }
            msg.Ack()
        }
    }()

    return nil
}

// Inject publishes a message into the internal loop
func (l *InternalLoop) Inject(ctx context.Context, msg *Message) error {
    return l.channel.Publish(ctx, msg)
}

// Publisher returns a Publisher adapter for the internal loop
func (l *InternalLoop) Publisher() Publisher {
    return NewChannelPublisher(l.channel)
}

// Subscriber returns a Subscriber adapter for specific topics
func (l *InternalLoop) Subscriber(topics ...string) Subscriber {
    return NewChannelSubscriber(l.channel, topics...)
}
```

### 5. External Break-Out

Messages can "break out" of the internal loop by specifying a non-internal destination:

```go
// Handler that breaks out to external system
func handleNotification(ctx context.Context, msg *Message) ([]*Message, error) {
    event := msg.Data.(OrderShippedEvent)

    // Internal routing continues
    internalMsg := message.MustNew(AuditEvent{...}, message.Attributes{
        AttrDestination: "gopipe://audit",  // Stays internal
        // ...
    })

    // Break out to external Kafka
    externalMsg := message.MustNew(NotificationEvent{...}, message.Attributes{
        AttrDestination: "kafka://notifications/order-updates",  // External
        // ...
    })

    return []*Message{internalMsg, externalMsg}, nil
}
```

### 6. External Sender Integration

```go
// ExternalDispatcher routes messages to appropriate external senders
type ExternalDispatcher struct {
    senders map[string]Sender  // scheme -> sender
}

func (d *ExternalDispatcher) Register(scheme string, sender Sender) {
    d.senders[scheme] = sender
}

func (d *ExternalDispatcher) Dispatch(ctx context.Context, msg *Message) error {
    dest := msg.Destination()
    u, _ := url.Parse(dest)

    sender, ok := d.senders[u.Scheme]
    if !ok {
        return fmt.Errorf("no sender for scheme: %s", u.Scheme)
    }

    return sender.Send(ctx, []*Message{msg})
}
```

### 7. Complete Pipeline Flow

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                    gopipe Internal Loop                      │
                    │                                                              │
External ──────────>│  ┌─────────┐     ┌─────────┐     ┌─────────┐               │
Input               │  │ Handler │ ──> │ Handler │ ──> │ Handler │               │
(HTTP/Kafka)        │  │ orders  │     │shipping │     │ audit   │               │
        │           │  └────┬────┘     └────┬────┘     └────┬────┘               │
        │           │       │               │               │                     │
        │           │       ▼               ▼               ▼                     │
        │           │  gopipe://        gopipe://       gopipe://                  │
        │           │  shipping         audit          complete                   │
        │           │       │               │               │                     │
        │           │       └───────────────┴───────────────┘                     │
        │           │                       │                                      │
        │           │              ┌────────┴────────┐                            │
        │           │              │  NoopChannel    │                            │
        │           │              │  (Go channels)  │                            │
        │           │              └────────┬────────┘                            │
        │           │                       │                                      │
        │           └───────────────────────┼──────────────────────────────────────┘
        │                                   │
        │                                   │ Break-out: kafka://...
        │                                   ▼
        │                          ┌────────────────┐
        └─────────────────────────>│ External       │──────────> External
                                   │ Dispatcher     │            Output
                                   └────────────────┘
```

## Rationale

1. **Pluggable Transport**: NoopChannel for simple cases, NATS for advanced
2. **Unified Interface**: MessageChannel abstracts underlying implementation
3. **Adapter Pattern**: Bridges to existing Publisher/Subscriber interfaces
4. **Clear Boundaries**: `gopipe://` destinations stay internal, others break out
5. **Full Pipeline**: Complete messaging without external dependencies

## Consequences

### Positive

- True composable pipelines with feedback loops
- Swap transport without changing handlers
- Clear internal/external boundary
- Works with Go channels (zero dependencies) or NATS (advanced features)
- Integrates with existing pub/sub infrastructure

### Negative

- New abstraction layer (MessageChannel)
- Routing logic complexity
- Potential for message loops if not careful

### Mitigations

- Provide loop detection (max hops, TTL)
- Clear documentation on internal vs external routing
- Default to NoopChannel for simplicity

## Links

- [ADR 0022: Internal Message Routing](0022-internal-message-routing.md)
- [ADR 0024: Destination Attribute](0024-destination-attribute.md)
- [Feature 13: Internal Message Loop](../features/13-internal-message-loop.md)
- [Feature 14: NATS Integration](../features/14-nats-integration.md)
