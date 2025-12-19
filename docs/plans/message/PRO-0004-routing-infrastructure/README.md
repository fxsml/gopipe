# PRO-0004: Routing Infrastructure

**Status:** Proposed
**Priority:** High
**Depends On:** PRO-0003-message-standardization
**Related ADRs:** 0022, 0023, 0024
**Related Features:** 12, 13

## Overview

This layer enables composable internal pipelines with clear routing semantics. Messages can flow internally via `gopipe://` destinations or break out to external systems via scheme-based routing.

## Goals

1. Implement topic-based internal routing
2. Add destination attribute with URI schemes
3. Create internal message loop with pluggable transport
4. Enable feedback loops without external dependencies

## Prerequisites

Layer 1 must be complete:
- [ ] CloudEvents validation enabled
- [ ] Non-generic Message implemented
- [ ] Serialization at boundaries working

## Sub-Tasks

### Task 2.1: Destination Attribute

**Goal:** Add `destination` attribute for explicit routing

**URI Schemes:**
```
gopipe://orders           → Internal handler
gopipe://shipping/cmds    → Internal with path
kafka://cluster/topic     → External Kafka
nats://subject            → External NATS
http://api.example.com    → External HTTP
```

**Implementation:**
```go
// message/destination.go
const AttrDestination = "destination"

type DestinationScheme string

const (
    SchemeGopipe DestinationScheme = "gopipe"
    SchemeKafka  DestinationScheme = "kafka"
    SchemeNATS   DestinationScheme = "nats"
    SchemeHTTP   DestinationScheme = "http"
    SchemeHTTPS  DestinationScheme = "https"
)

type Destination struct {
    Scheme DestinationScheme
    Host   string
    Path   string
    Query  url.Values
}

func ParseDestination(dest string) (*Destination, error) {
    u, err := url.Parse(dest)
    if err != nil {
        return nil, err
    }
    return &Destination{
        Scheme: DestinationScheme(u.Scheme),
        Host:   u.Host,
        Path:   strings.TrimPrefix(u.Path, "/"),
        Query:  u.Query(),
    }, nil
}

func (d *Destination) IsInternal() bool {
    return d.Scheme == SchemeGopipe
}

// Message helpers
func (m *Message) Destination() string {
    dest, _ := m.Attributes[AttrDestination].(string)
    return dest
}

func (m *Message) SetDestination(dest string) {
    m.Attributes[AttrDestination] = dest
}

func (m *Message) IsInternalDestination() bool {
    return strings.HasPrefix(m.Destination(), "gopipe://")
}
```

**Files to Create:**
- `message/destination.go` - Destination parsing
- `message/destination_test.go` - Tests

---

### Task 2.2: Topic vs Destination Clarification

**Goal:** Clarify the semantic difference

| Attribute | Purpose | Example |
|-----------|---------|---------|
| `topic` | Semantic grouping (what kind) | `"orders"`, `"payments"` |
| `destination` | Physical routing (where to) | `"gopipe://orders"`, `"kafka://prod/orders"` |

**Usage:**
```go
msg := message.MustNew(order, message.Attributes{
    message.AttrType:        "order.created",
    message.AttrTopic:       "orders",              // Semantic: it's an order event
    message.AttrDestination: "gopipe://processing", // Physical: route to processing
})
```

**Router Priority:**
1. Check `destination` attribute first
2. Fall back to `topic` for handler selection
3. Fall back to `type` for handler selection

---

### Task 2.3: InternalRouter

**Goal:** Route messages by topic/type without external brokers

**Implementation:**
```go
// message/internal_router.go
type InternalRouter struct {
    routes     map[string][]Handler   // topic → handlers
    typeRoutes map[string][]Handler   // type → handlers
    config     InternalRouterConfig
}

type InternalRouterConfig struct {
    BufferSize int
    OnNoMatch  func(msg *Message)
}

func (r *InternalRouter) Route(topic string, handler Handler)
func (r *InternalRouter) RouteType(eventType string, handler Handler)
func (r *InternalRouter) RouteFunc(topic string, fn HandlerFunc)

func (r *InternalRouter) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    // 1. Try topic-based routing
    topic := msg.Topic()
    if handlers, ok := r.routes[topic]; ok {
        return r.dispatch(ctx, msg, handlers)
    }

    // 2. Fall back to type-based routing
    msgType := msg.Type()
    if handlers, ok := r.typeRoutes[msgType]; ok {
        return r.dispatch(ctx, msg, handlers)
    }

    // 3. No match
    if r.config.OnNoMatch != nil {
        r.config.OnNoMatch(msg)
    }
    return nil, ErrNoMatchingHandler
}
```

---

### Task 2.4: MessageChannel Interface

**Goal:** Abstract internal pub/sub transport

**Interface:**
```go
// message/channel.go
type MessageChannel interface {
    Publish(ctx context.Context, msg *Message) error
    Subscribe(ctx context.Context, topics ...string) (<-chan *Message, error)
    Close() error
}

type MessageChannelConfig struct {
    BufferSize     int
    DefaultTimeout time.Duration
}
```

**NoopChannel (Go channels):**
```go
// message/channel_noop.go
type NoopChannel struct {
    mu         sync.RWMutex
    topics     map[string][]chan *Message
    bufferSize int
    closed     bool
}

func NewNoopChannel(config MessageChannelConfig) *NoopChannel

func (c *NoopChannel) Publish(ctx context.Context, msg *Message) error {
    dest := msg.Destination()
    topic, err := parseInternalDestination(dest)
    if err != nil {
        return err
    }

    c.mu.RLock()
    defer c.mu.RUnlock()

    for _, sub := range c.topics[topic] {
        select {
        case sub <- msg:
        case <-ctx.Done():
            return ctx.Err()
        default:
            // Buffer full
        }
    }
    return nil
}

func (c *NoopChannel) Subscribe(ctx context.Context, topics ...string) (<-chan *Message, error) {
    c.mu.Lock()
    defer c.mu.Unlock()

    ch := make(chan *Message, c.bufferSize)
    for _, topic := range topics {
        c.topics[topic] = append(c.topics[topic], ch)
    }
    return ch, nil
}
```

---

### Task 2.5: InternalLoop

**Goal:** Complete feedback loop with external break-out

**Implementation:**
```go
// message/loop.go
type InternalLoop struct {
    channel      MessageChannel
    router       *InternalRouter
    external     *ExternalDispatcher
    config       InternalLoopConfig
}

type InternalLoopConfig struct {
    BufferSize     int
    OnLoopOverflow func(msg *Message)
}

func NewInternalLoop(opts ...InternalLoopOption) *InternalLoop

func (l *InternalLoop) Route(path string, handler Handler) {
    l.router.Route(path, handler)
}

func (l *InternalLoop) Start(ctx context.Context) error {
    topics := l.router.Topics()
    msgs, _ := l.channel.Subscribe(ctx, topics...)

    for msg := range msgs {
        outputs, err := l.router.Handle(ctx, msg)
        if err != nil {
            msg.Nack(err)
            continue
        }

        for _, out := range outputs {
            if out.IsInternalDestination() {
                l.channel.Publish(ctx, out)
            } else {
                l.external.Dispatch(ctx, out)
            }
        }
        msg.Ack()
    }
    return nil
}

func (l *InternalLoop) Inject(ctx context.Context, msg *Message) error {
    return l.channel.Publish(ctx, msg)
}
```

---

### Task 2.6: ExternalDispatcher

**Goal:** Route non-internal messages to external senders

**Implementation:**
```go
// message/dispatcher.go
type ExternalDispatcher struct {
    senders map[DestinationScheme]Sender
}

func (d *ExternalDispatcher) Register(scheme DestinationScheme, sender Sender)

func (d *ExternalDispatcher) Dispatch(ctx context.Context, msg *Message) error {
    dest, _ := ParseDestination(msg.Destination())
    sender, ok := d.senders[dest.Scheme]
    if !ok {
        return fmt.Errorf("no sender for scheme: %s", dest.Scheme)
    }
    return sender.Send(ctx, []*Message{msg})
}
```

---

## Architecture

```
┌────────────────────────────────────────────────────────────────────────────┐
│                           Internal Loop                                     │
│                                                                             │
│  External Input ──────────────────────────────────────────────────────┐     │
│  (HTTP/Kafka)                                                         │     │
│       │                                                               │     │
│       ▼                                                               │     │
│  ┌─────────────────────────────────────────────────────────────────┐ │     │
│  │                    MessageChannel (NoopChannel)                  │ │     │
│  │                                                                  │ │     │
│  │   gopipe://        gopipe://        gopipe://                    │ │     │
│  │   orders           shipping         audit                        │ │     │
│  │      │                │                │                         │ │     │
│  └──────┼────────────────┼────────────────┼─────────────────────────┘ │     │
│         │                │                │                           │     │
│         ▼                ▼                ▼                           │     │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐                   │     │
│  │  Handler   │   │  Handler   │   │  Handler   │                   │     │
│  │  orders    │   │  shipping  │   │  audit     │                   │     │
│  └─────┬──────┘   └─────┬──────┘   └─────┬──────┘                   │     │
│        │                │                │                           │     │
│        ▼                ▼                ▼                           │     │
│   gopipe://        gopipe://        kafka://                         │     │
│   shipping         audit           notifications ───────────────────┼─────┤
│        │                │                                            │     │
│        └────────────────┘                                            │     │
│               │                                                      │     │
│               │ (feedback via MessageChannel)                        │     │
│               ▼                                                      │     │
│        Back to topics                                                │     │
│                                                                      │     │
└──────────────────────────────────────────────────────────────────────┼─────┘
                                                                       │
                                                                       ▼
                                                              External Systems
```

## Implementation Order

```
1. Destination Attribute ──────────────────┐
                                           │
2. Topic vs Destination Clarification ─────┼──► 3. InternalRouter
                                           │
                                           └──► 4. MessageChannel ──► 5. InternalLoop
                                                                              │
                                                                              ▼
                                                                    6. ExternalDispatcher
```

**Recommended PR Sequence:**
1. **PR 1:** Destination Attribute + Helpers
2. **PR 2:** InternalRouter
3. **PR 3:** MessageChannel + NoopChannel
4. **PR 4:** InternalLoop + ExternalDispatcher

## Validation Checklist

Before marking Layer 2 complete:

- [ ] `destination` attribute implemented
- [ ] URI parsing works for all schemes
- [ ] `IsInternalDestination()` helper works
- [ ] InternalRouter routes by topic, type
- [ ] NoopChannel implements MessageChannel
- [ ] InternalLoop handles feedback
- [ ] External break-out via dispatcher
- [ ] No circular imports
- [ ] All tests pass
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0022: Internal Message Routing](../adr/0022-internal-message-routing.md)
- [ADR 0023: Internal Message Loop](../adr/0023-internal-message-loop.md)
- [ADR 0024: Destination Attribute](../adr/0024-destination-attribute.md)
- [Feature 12: Internal Message Routing](../features/12-internal-message-routing.md)
- [Feature 13: Internal Message Loop](../features/13-internal-message-loop.md)
