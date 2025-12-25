# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Marshaler)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and type-based routing.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            message.Engine                                 │
│                                                                           │
│  ┌───────────┐   ┌───────────┐   ┌────────┐   ┌────────┐   ┌─────────┐   │
│  │Subscribers│──>│ Unmarshal │──>│ FanIn  │──>│ Router │──>│ Handler │   │
│  │ ([]byte)  │   │  (→ any)  │   │        │   │        │   │         │   │
│  └───────────┘   └───────────┘   └────────┘   └────────┘   └────┬────┘   │
│                                       ↑                         │        │
│                                       │                         ▼        │
│                                       │    ┌───────────┐   ┌────────┐   │
│                                       │    │Generators │──>│ FanOut │   │
│                                       │    │  (typed)  │   │        │   │
│                                       │    └───────────┘   └───┬────┘   │
│                                       │                    /   │        │
│                                       │      internal     /    │external│
│                                       │      (typed)     /     ▼        │
│                                       └─────────────────┘  ┌────────┐   │
│                                                            │Marshal │   │
│                                                            └───┬────┘   │
│                                                                ▼        │
│                                                           ┌─────────┐   │
│                                                           │Publisher│   │
│                                                           │([]byte) │   │
│                                                           └─────────┘   │
└──────────────────────────────────────────────────────────────────────────┘
```

Key points:
- **Subscribers** produce `[]byte`, go through Unmarshal before FanIn
- **Generators** produce typed data, feed directly into FanOut
- **FanOut** routes: internal (typed, back to FanIn) or external (marshal → publisher)
- **Internal loop** skips marshal/unmarshal (stays typed)

## Implementation Phases

### Phase 1: Minimal Engine

Single subscriber, single handler, single publisher. No FanIn/FanOut, no internal loop.

Type registry is auto-generated from handlers on `Start()` - no manual registration needed.

```go
marshaler := message.NewJSONMarshaler()  // No Register() calls needed!

engine := message.NewEngine(message.EngineConfig{})
_ = engine.SetMarshaler(marshaler)
_ = engine.AddSubscriber("nats", natsSubscriber)
_ = engine.AddPublisher("nats", natsPublisher)
_ = engine.AddHandler("process-order", NewHandler[OrderCreated](fn))
done, _ := engine.Start(ctx)  // Returns done channel, closed on completion

// Subscribe to topics (can be done before or after Start)
_ = engine.Subscribe(ctx, "nats", "orders")

// Type registry built automatically from handler's EventType()
<-done  // Wait for engine to stop
```

### Phase 2: Multiple Handlers

Add type-based dispatch to multiple handlers.

### Phase 3: Internal Loopback

Enable `gopipe://` destinations to route back internally.

### Phase 4: FanIn/FanOut

Multiple subscribers merged, multiple publishers by destination pattern.

---

## Core Types

### Handler Interface

```go
// Handler - non-generic interface for Router
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

**Design Decision: EventType vs Predicate**

`EventType()` is **capability**, not filtering policy:
- Handler receives `event any` but casts to specific type
- Wrong type = handler fails with type assertion error
- Therefore EventType describes what the handler *can* handle

Additional filtering (source, subject, CESQL) is **policy**:
- "Should I handle this specific message?" is a separate concern
- Implemented via separate functions (out of scope for Phase 1):
  - `AddHandlerWithFilter(name, handler, predicate)`
  - `AddHandlerWithCESQL(name, handler, "source = 'order-service'")`

Router flow with filtering:
1. Lookup handlers by EventType → O(1)
2. For matched handlers, evaluate optional filter → O(m)
3. Execute matching handlers

### Generic Handler Constructor

```go
// NewHandler creates a handler for type T
// Type T determines routing (EventType), handler receives full Message
func NewHandler[T any](
    fn func(ctx context.Context, msg *Message) ([]*Message, error),
) Handler {
    return &typedHandler[T]{
        fn:        fn,
        eventType: reflect.TypeOf((*T)(nil)).Elem(),
    }
}

type typedHandler[T any] struct {
    fn        func(ctx context.Context, msg *Message) ([]*Message, error)
    eventType reflect.Type
}

func (h *typedHandler[T]) EventType() reflect.Type {
    return h.eventType
}

func (h *typedHandler[T]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    // Type already validated by Router before calling Handle
    return h.fn(ctx, msg)
}
```

**Future: CQRS-style convenience constructors**

```go
// NewCQRSHandler for pure domain handlers
// Handler function has no messaging knowledge - just domain types
// Optional attrsFn generates message attributes from input/output
func NewCQRSHandler[In, Out any](
    fn func(ctx context.Context, event *In) (*Out, error),
    attrsFn ...func(in *In, out *Out) Attributes,
) Handler {
    return NewHandler[In](func(ctx context.Context, msg *Message) ([]*Message, error) {
        in := msg.Data.(*In)
        out, err := fn(ctx, in)
        if err != nil {
            return nil, err
        }
        if out == nil {
            return nil, nil
        }
        var attrs Attributes
        if len(attrsFn) > 0 && attrsFn[0] != nil {
            attrs = attrsFn[0](in, out)
        }
        return []*Message{New(out, attrs)}, nil
    })
}

// Usage - pure domain handler (no messaging knowledge):
engine.AddHandler("ship-order", NewCQRSHandler(
    func(ctx context.Context, order *OrderCreated) (*OrderShipped, error) {
        return &OrderShipped{
            OrderID:    order.OrderID,
            TrackingID: uuid.NewString(),
        }, nil
    },
))

// With attribute generator (messaging concern separated):
engine.AddHandler("ship-order", NewCQRSHandler(
    func(ctx context.Context, order *OrderCreated) (*OrderShipped, error) {
        return &OrderShipped{
            OrderID:    order.OrderID,
            TrackingID: uuid.NewString(),
        }, nil
    },
    func(in *OrderCreated, out *OrderShipped) Attributes {
        return Attributes{
            Subject:     in.OrderID,
            Destination: "kafka://shipments",
        }
    },
))
```

### Router

```go
type Router struct {
    mu        sync.RWMutex
    handlers  map[reflect.Type][]namedHandler
    marshaler Marshaler
}

type namedHandler struct {
    name    string
    handler Handler
}

func NewRouter(marshaler Marshaler) *Router {
    return &Router{
        handlers:  make(map[reflect.Type][]namedHandler),
        marshaler: marshaler,
    }
}

func (r *Router) AddHandler(name string, h Handler) {
    r.mu.Lock()
    defer r.mu.Unlock()
    t := h.EventType()
    r.handlers[t] = append(r.handlers[t], namedHandler{name: name, handler: h})
}

func (r *Router) Route(ctx context.Context, msg *Message) ([]*Message, error) {
    // Get CE type from message
    ceType := msg.Type()

    // Lookup Go type
    goType, ok := r.marshaler.TypeFor(ceType)
    if !ok {
        return nil, fmt.Errorf("unknown type: %s", ceType)
    }

    // Get handlers
    r.mu.RLock()
    handlers := r.handlers[goType]
    r.mu.RUnlock()

    if len(handlers) == 0 {
        return nil, fmt.Errorf("no handler for type: %s", goType)
    }

    // Execute handlers
    var results []*Message
    for _, nh := range handlers {
        out, err := nh.handler.Handle(ctx, msg)
        if err != nil {
            return nil, err
        }
        results = append(results, out...)
    }

    return results, nil
}
```

### Subscriber / Generator / Publisher Interfaces

```go
// Subscriber is an external source with topic-based subscription
type Subscriber interface {
    Subscribe(ctx context.Context, topic string) (<-chan *Message, error)
}

// Generator is an internal source (no topic concept)
type Generator interface {
    Generate(ctx context.Context) (<-chan *Message, error)
}

// Publisher sends messages to external destination
type Publisher interface {
    Publish(ctx context.Context, msgs <-chan *Message) error
}

type Pipe interface {
    Pipe(ctx context.Context, msgs <-chan *Message) (<-chan *Message, error)
}

type Starter interface {
    Start(ctx context.Context) (<-chan struct{}, error)
}
```

### Named Subscribers

Register subscribers by name, then subscribe to topics:

```go
// Register subscribers
engine.AddSubscriber("nats", natsSubscriber)
engine.AddSubscriber("redis", redisSubscriber)

// Subscribe to topics (variadic)
engine.Subscribe(ctx, "nats", "orders", "payments")
engine.Subscribe(ctx, "redis", "cache:expired")

// Unsubscribe
engine.Unsubscribe("nats", "orders")
```

### Engine

```go
type Engine struct {
    marshaler   Marshaler
    router      *Router
    config      EngineConfig

    subscribers map[string]Subscriber         // Named subscribers
    publishers  map[string]Publisher          // Named publishers (future)
    generators  map[string]*generatorEntry    // Active generators
    topics      map[string]*topicEntry        // Active topic subscriptions
    fanIn       *channel.DynamicFanIn         // Merges subscriber channels
    fanOut      chan *Message                 // Handler output + generators

    started bool
    mu      sync.Mutex
}

type generatorEntry struct {
    gen    Generator
    cancel context.CancelFunc
}

type topicEntry struct {
    subscriber string                // Name of subscriber
    cancel     context.CancelFunc
}

type EngineConfig struct {
    Concurrency      int
    BufferSize       int
    InternalLoopback bool
}

func NewEngine(config EngineConfig) *Engine {
    bufSize := config.BufferSize
    if bufSize == 0 {
        bufSize = 100
    }
    return &Engine{
        config:      config,
        subscribers: make(map[string]Subscriber),
        publishers:  make(map[string]Publisher),
        generators:  make(map[string]*generatorEntry),
        topics:      make(map[string]*topicEntry),
        fanIn:       channel.NewDynamicFanIn(),
        fanOut:      make(chan *Message, bufSize),
    }
}

func (e *Engine) SetMarshaler(m Marshaler) error {
    if e.started {
        return errors.New("cannot set marshaler after start")
    }
    e.marshaler = m
    e.router = NewRouter(m)
    return nil
}

func (e *Engine) AddSubscriber(name string, sub Subscriber) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    if _, exists := e.subscribers[name]; exists {
        return fmt.Errorf("subscriber %q already exists", name)
    }
    e.subscribers[name] = sub
    return nil
}

func (e *Engine) AddPublisher(name string, pub Publisher) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    if _, exists := e.publishers[name]; exists {
        return fmt.Errorf("publisher %q already exists", name)
    }
    e.publishers[name] = pub
    return nil
}

func (e *Engine) AddHandler(name string, h Handler) error {
    if e.started {
        return errors.New("cannot add handler after start")
    }
    e.router.AddHandler(name, h)
    return nil
}

// Subscribe subscribes to topics using a named subscriber
func (e *Engine) Subscribe(ctx context.Context, subscriberName string, topics ...string) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    sub, exists := e.subscribers[subscriberName]
    if !exists {
        return fmt.Errorf("subscriber %q not found", subscriberName)
    }

    for _, topic := range topics {
        key := subscriberName + ":" + topic
        if _, exists := e.topics[key]; exists {
            continue // Already subscribed
        }

        topicCtx, cancel := context.WithCancel(ctx)
        ch, err := sub.Subscribe(topicCtx, topic)
        if err != nil {
            cancel()
            return err
        }

        e.topics[key] = &topicEntry{subscriber: subscriberName, cancel: cancel}

        // Unmarshal subscriber output, then add to FanIn
        unmarshaled := e.unmarshalStream(topicCtx, ch)
        e.fanIn.Add(unmarshaled)
    }
    return nil
}

// AddGenerator adds a generator that feeds into FanOut (already typed)
func (e *Engine) AddGenerator(ctx context.Context, name string, gen Generator) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    if _, exists := e.generators[name]; exists {
        return fmt.Errorf("generator %q already exists", name)
    }

    ch, err := gen.Generate(ctx)
    if err != nil {
        return err
    }

    e.generators[name] = &sourceEntry{source: gen, ctx: ctx}

    // Generator output goes directly to FanOut (no unmarshal needed)
    go func() {
        for msg := range ch {
            e.fanOut <- msg
        }
    }()
    return nil
}

// Unsubscribe unsubscribes from topics
func (e *Engine) Unsubscribe(subscriberName string, topics ...string) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    for _, topic := range topics {
        key := subscriberName + ":" + topic
        entry, exists := e.topics[key]
        if !exists {
            continue
        }
        entry.cancel() // Stop the subscription
        delete(e.topics, key)
    }
    return nil
}

func (e *Engine) RemoveGenerator(name string) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    // Source stops when its context is cancelled by caller
    delete(e.generators, name)
    return nil
}

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error) {
    e.mu.Lock()
    if e.started {
        e.mu.Unlock()
        return nil, errors.New("already started")
    }
    if len(e.publishers) == 0 {
        e.mu.Unlock()
        return nil, errors.New("no publisher set")
    }
    e.started = true
    e.mu.Unlock()

    // Auto-generate type registry from handlers
    e.buildTypeRegistry()

    // Create output channel for publisher(s)
    output := make(chan *Message, e.config.BufferSize)
    done := make(chan struct{})

    // Start all publishers (future: route by destination pattern)
    for _, pub := range e.publishers {
        go func(p Publisher) {
            _ = p.Publish(ctx, output)
        }(pub)
    }

    // Router loop: FanIn → Router → Handler → FanOut
    go func() {
        for msg := range e.fanIn.Output() {
            e.processRouted(ctx, msg)
        }
    }()

    // FanOut loop: route internal (back to FanIn) or external (marshal → publisher)
    go func() {
        defer close(done)
        defer close(output)
        for msg := range e.fanOut {
            if isInternalDest(msg.Destination()) {
                // Internal: back to FanIn (stays typed)
                e.fanIn.Inject(msg)
            } else {
                // External: marshal and send to publisher
                e.marshal(msg)
                output <- msg
            }
        }
    }()

    return done, nil
}

// buildTypeRegistry auto-generates the marshaler's type registry
// from the handlers' EventType() methods. No manual registration needed.
func (e *Engine) buildTypeRegistry() {
    for goType := range e.router.handlers {
        // Derive CE type name from Go type using naming strategy
        ceType := e.marshaler.Name(reflect.New(goType).Elem().Interface())
        e.marshaler.Register(ceType, reflect.New(goType).Elem().Interface())
    }
}

// processRouted handles messages from FanIn (already unmarshaled/typed)
func (e *Engine) processRouted(ctx context.Context, msg *Message) {
    // Route to handlers
    outputs, err := e.router.Route(ctx, msg)
    if err != nil {
        return
    }

    // Send handler outputs to FanOut
    for _, out := range outputs {
        e.fanOut <- out
    }
}

func isInternalDest(dest string) bool {
    return strings.HasPrefix(dest, "gopipe://")
}
```

---

## Usage Example

```go
package main

type OrderCreated struct {
    OrderID string  `json:"order_id"`
    Amount  float64 `json:"amount"`
}

type OrderShipped struct {
    OrderID    string `json:"order_id"`
    TrackingID string `json:"tracking_id"`
}

func main() {
    ctx := context.Background()

    // Setup marshaler (no manual type registration needed!)
    marshaler := message.NewJSONMarshaler()

    // Create subscriber/publisher (from your broker adapters)
    natsSubscriber := nats.NewSubscriber(natsConfig)
    natsPublisher := nats.NewPublisher(natsConfig)

    // Create engine
    engine := message.NewEngine(message.EngineConfig{
        BufferSize: 100,
    })
    _ = engine.SetMarshaler(marshaler)
    _ = engine.AddSubscriber("nats", natsSubscriber)
    _ = engine.AddPublisher("nats", natsPublisher)

    // Option 1: Message-centric handler (default)
    _ = engine.AddHandler("ship-order", message.NewHandler[OrderCreated](
        func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            order := msg.Data.(*OrderCreated)
            return []*message.Message{
                message.New(OrderShipped{
                    OrderID:    order.OrderID,
                    TrackingID: uuid.NewString(),
                }, message.Attributes{Destination: "nats://shipments"}),
            }, nil
        },
    ))

    // Option 2: CQRS-style handler (future) - pure domain function
    // _ = engine.AddHandler("ship-order", message.NewCQRSHandler(
    //     func(ctx context.Context, order *OrderCreated) (*OrderShipped, error) {
    //         return &OrderShipped{OrderID: order.OrderID, TrackingID: uuid.NewString()}, nil
    //     },
    //     func(in *OrderCreated, out *OrderShipped) message.Attributes {
    //         return message.Attributes{Destination: "nats://shipments"}
    //     },
    // ))

    done, _ := engine.Start(ctx)

    // Subscribe to topics
    _ = engine.Subscribe(ctx, "nats", "orders")

    // Wait for engine to complete
    <-done
}
```

---

## Future Phases

### Phase 3: Multiple Publishers

Route to different publishers based on destination pattern:

```go
// Multiple publishers by destination pattern
_ = engine.AddPublisher("kafka://", kafkaPublisher)
_ = engine.AddPublisher("http://", httpPublisher)

// Handler output with destination:
return []*message.Message{
    message.New(event, message.Attributes{}).WithDestination("kafka://orders"),
    message.New(event, message.Attributes{}).WithDestination("http://webhook"),
}
```

### Phase 4: Handler Filtering (CESQL)

Advanced handler registration with CloudEvents SQL filtering:

```go
// Filter on CloudEvents attributes (after type matching)
engine.AddHandlerWithCESQL("priority-orders", handler,
    "source = 'priority-service' AND subject LIKE 'urgent.%'")

// Or with predicate function
engine.AddHandlerWithFilter("vip-orders", handler,
    func(msg *Message) bool {
        return msg.Source() == "vip-service"
    })
```

Note: Type matching (EventType) always happens first. CESQL/predicate filters
are evaluated only for type-matched handlers.

---

## Generators

### Ticker Generator

```go
// Ticker generates messages at intervals
type Ticker struct {
    Interval time.Duration
    Factory  func() *Message
}

func (t *Ticker) Start(ctx context.Context) (<-chan *Message, error) {
    out := make(chan *Message)
    go func() {
        defer close(out)
        ticker := time.NewTicker(t.Interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                out <- t.Factory()
            }
        }
    }()
    return out, nil
}
```

### Usage

```go
ticker := &message.Ticker{
    Interval: 30 * time.Second,
    Factory: func() *Message {
        return message.New(HealthCheck{Time: time.Now()}, message.Attributes{})
    },
}
_ = engine.AddGenerator(ctx, "healthcheck", ticker)
```

---

## Leader Election Example

```go
engine := message.NewEngine(config)
_ = engine.SetMarshaler(marshaler)
_ = engine.AddSubscriber("nats", natsSubscriber)
_ = engine.AddPublisher("nats", natsPublisher)
_ = engine.AddHandler(handler)
done, _ := engine.Start(ctx)

election.OnBecomeLeader(func() {
    // Subscribe only when we are leader
    _ = engine.Subscribe(ctx, "nats", "orders")
})

election.OnLoseLeadership(func() {
    // Unsubscribe when we lose leadership
    _ = engine.Unsubscribe("nats", "orders")
})
```

---

## Test Plan

1. Single handler receives typed data
2. Multiple handlers for same type all execute
3. Unknown type returns error
4. Marshal/unmarshal round-trip preserves data
5. Internal loop routes correctly (Phase 3)
6. AddSource adds source after Start
7. RemoveSource stops receiving from source
8. Source context cancellation stops source
9. Generator produces messages at expected intervals
10. Multiple sources merge correctly via FanIn
