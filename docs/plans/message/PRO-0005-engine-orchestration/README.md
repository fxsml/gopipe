# PRO-0005: Engine Orchestration

**Status:** Proposed
**Priority:** Medium (builds on solid foundation)
**Depends On:** PRO-0004-routing-infrastructure
**Related ADRs:** 0029
**Related Features:** 17

## Overview

The Message Engine is the top-level orchestration component that declaratively wires subscribers, routers, and publishers. It handles internal feedback loops automatically and provides a clean separation between orchestration and processing concerns.

## Goals

1. Provide declarative component registration
2. Implement FanIn for merging subscriber outputs
3. Implement RoutingFanOut for destination-based routing
4. Handle internal loops via `gopipe://` destination
5. Preserve simple mode for basic use cases

## Prerequisites

Layer 2 must be complete:
- [ ] Destination attribute implemented
- [ ] InternalRouter working
- [ ] MessageChannel interface defined
- [ ] InternalLoop functional

## Sub-Tasks

### Task 3.1: RoutingFanIn

**Goal:** Merge multiple subscriber outputs into a single channel

**Implementation:**
```go
// message/fanin.go
type RoutingFanIn struct {
    inputs  map[string]<-chan *Message
    output  chan *Message
    config  RoutingFanInConfig
}

type RoutingFanInConfig struct {
    BufferSize    int
    PreserveOrder bool  // false = concurrent, true = round-robin
}

func NewRoutingFanIn(config RoutingFanInConfig) *RoutingFanIn {
    return &RoutingFanIn{
        inputs: make(map[string]<-chan *Message),
        output: make(chan *Message, config.BufferSize),
        config: config,
    }
}

func (f *RoutingFanIn) AddInput(name string, in <-chan *Message) {
    f.inputs[name] = in
}

func (f *RoutingFanIn) Output() <-chan *Message {
    return f.output
}

func (f *RoutingFanIn) Start(ctx context.Context) {
    var wg sync.WaitGroup
    for name, input := range f.inputs {
        wg.Add(1)
        go func(name string, ch <-chan *Message) {
            defer wg.Done()
            for msg := range ch {
                select {
                case f.output <- msg:
                case <-ctx.Done():
                    return
                }
            }
        }(name, input)
    }
    go func() {
        wg.Wait()
        close(f.output)
    }()
}
```

---

### Task 3.2: RoutingFanOut for Engine

**Goal:** Route messages to publishers by destination scheme

**Implementation:**
```go
// message/engine_fanout.go (builds on channel/routing_fanout.go)
type EngineFanOut struct {
    outputs       map[string]chan<- *Message  // pattern → output channel
    loopOutput    chan<- *Message             // gopipe:// → loop
    deadLetter    chan<- *Message             // unroutable
    config        EngineFanOutConfig
}

type EngineFanOutConfig struct {
    BufferPerOutput int
    OnNoMatch       func(msg *Message)
    OnSlowConsumer  func(pattern string, msg *Message)
}

func (f *EngineFanOut) Route(ctx context.Context, msg *Message) error {
    dest := msg.Destination()

    // Internal loop
    if strings.HasPrefix(dest, "gopipe://") {
        return f.sendTo(ctx, f.loopOutput, msg)
    }

    // Match by scheme prefix
    for pattern, output := range f.outputs {
        if strings.HasPrefix(dest, pattern) {
            return f.sendTo(ctx, output, msg)
        }
    }

    // No match - dead letter or callback
    if f.config.OnNoMatch != nil {
        f.config.OnNoMatch(msg)
    }
    if f.deadLetter != nil {
        return f.sendTo(ctx, f.deadLetter, msg)
    }
    return ErrNoMatchingDestination
}
```

---

### Task 3.3: Engine Core

**Goal:** Main orchestrator struct with registration APIs

**Implementation:**
```go
// message/engine.go
type Engine struct {
    mu          sync.Mutex
    subscribers map[string]Subscriber[*Message]  // name → subscriber
    routers     map[string]*Router               // source → router
    publishers  map[string]Publisher             // pattern → publisher

    fanIn       *RoutingFanIn
    fanOut      *EngineFanOut
    loopChan    chan *Message

    sourceMatcher SourceMatcher
    config        EngineConfig
    state         EngineState
}

type EngineState int

const (
    EngineStateCreated EngineState = iota
    EngineStateStarting
    EngineStateRunning
    EngineStateStopping
    EngineStateStopped
)

type EngineConfig struct {
    OnSubscriberError func(name string, err error)
    OnRouterError     func(source string, msg *Message, err error)
    OnPublisherError  func(dest string, msg *Message, err error)
    OnLoopOverflow    func(msg *Message)
    DeadLetterDest    string
    ShutdownTimeout   time.Duration
    DrainTimeout      time.Duration
    LoopBufferSize    int
}

func NewEngine(config ...EngineConfig) *Engine {
    cfg := EngineConfig{LoopBufferSize: 1000}
    if len(config) > 0 {
        cfg = config[0]
    }
    return &Engine{
        subscribers:   make(map[string]Subscriber[*Message]),
        routers:       make(map[string]*Router),
        publishers:    make(map[string]Publisher),
        sourceMatcher: DefaultSourceMatcher,
        config:        cfg,
        state:         EngineStateCreated,
    }
}
```

---

### Task 3.4: Registration APIs

**Implementation:**
```go
// message/engine.go (continued)

func (e *Engine) AddSubscriber(name string, sub Subscriber[*Message]) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    if e.state != EngineStateCreated {
        return ErrEngineAlreadyStarted
    }
    if _, exists := e.subscribers[name]; exists {
        return ErrDuplicateSubscriber
    }
    e.subscribers[name] = sub
    return nil
}

func (e *Engine) AddRouter(source string, router *Router) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    if e.state != EngineStateCreated {
        return ErrEngineAlreadyStarted
    }
    if _, exists := e.routers[source]; exists {
        return ErrDuplicateRouter
    }
    e.routers[source] = router
    return nil
}

func (e *Engine) AddPublisher(pattern string, pub Publisher) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    if e.state != EngineStateCreated {
        return ErrEngineAlreadyStarted
    }
    if _, exists := e.publishers[pattern]; exists {
        return ErrDuplicatePublisher
    }
    e.publishers[pattern] = pub
    return nil
}

func (e *Engine) SetSourceMatcher(matcher SourceMatcher) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.sourceMatcher = matcher
}

// SourceMatcher determines which router handles a message
type SourceMatcher func(msg *Message) string

// DefaultSourceMatcher extracts source from type prefix
var DefaultSourceMatcher = func(msg *Message) string {
    typ := msg.Type()
    if idx := strings.Index(typ, "."); idx != -1 {
        return typ[:idx]
    }
    return "default"
}
```

---

### Task 3.5: Lifecycle Management

**Implementation:**
```go
// message/engine_lifecycle.go

func (e *Engine) Start(ctx context.Context) error {
    e.mu.Lock()
    if e.state != EngineStateCreated {
        e.mu.Unlock()
        return ErrEngineInvalidState
    }
    e.state = EngineStateStarting
    e.mu.Unlock()

    // 1. Create loop channel
    e.loopChan = make(chan *Message, e.config.LoopBufferSize)

    // 2. Start all subscribers and create FanIn
    e.fanIn = NewRoutingFanIn(RoutingFanInConfig{BufferSize: 100})
    for name, sub := range e.subscribers {
        ch := sub.Subscribe(ctx)
        e.fanIn.AddInput(name, ch)
    }
    // Add loop channel as input
    e.fanIn.AddInput("_loop", e.loopChan)
    e.fanIn.Start(ctx)

    // 3. Create FanOut with publisher outputs
    e.fanOut = &EngineFanOut{
        outputs:    make(map[string]chan<- *Message),
        loopOutput: e.loopChan,
        config: EngineFanOutConfig{
            BufferPerOutput: 100,
            OnNoMatch:       e.config.OnLoopOverflow,
        },
    }
    for pattern, pub := range e.publishers {
        pubChan := make(chan *Message, 100)
        e.fanOut.outputs[pattern] = pubChan
        go e.runPublisher(ctx, pattern, pub, pubChan)
    }

    // 4. Start main processing loop
    go e.processMessages(ctx)

    e.mu.Lock()
    e.state = EngineStateRunning
    e.mu.Unlock()

    return nil
}

func (e *Engine) processMessages(ctx context.Context) {
    for msg := range e.fanIn.Output() {
        // Match message to router
        source := e.sourceMatcher(msg)
        router, ok := e.routers[source]
        if !ok {
            router = e.routers["default"]
        }
        if router == nil {
            e.config.OnRouterError(source, msg, ErrNoMatchingRouter)
            continue
        }

        // Process through router
        outputs, err := router.Handle(ctx, msg)
        if err != nil {
            e.config.OnRouterError(source, msg, err)
            continue
        }

        // Route outputs
        for _, out := range outputs {
            if err := e.fanOut.Route(ctx, out); err != nil {
                e.config.OnPublisherError(out.Destination(), out, err)
            }
        }
    }
}

func (e *Engine) runPublisher(ctx context.Context, pattern string, pub Publisher, ch <-chan *Message) {
    for msg := range ch {
        if err := pub.Publish(ctx, msg); err != nil {
            e.config.OnPublisherError(pattern, msg, err)
        }
    }
}

func (e *Engine) Stop(ctx context.Context) error {
    e.mu.Lock()
    if e.state != EngineStateRunning {
        e.mu.Unlock()
        return ErrEngineInvalidState
    }
    e.state = EngineStateStopping
    e.mu.Unlock()

    // 1. Stop accepting from loop
    close(e.loopChan)

    // 2. Wait for drain with timeout
    timer := time.NewTimer(e.config.DrainTimeout)
    select {
    case <-timer.C:
    case <-ctx.Done():
    }

    e.mu.Lock()
    e.state = EngineStateStopped
    e.mu.Unlock()

    return nil
}

func (e *Engine) Health() EngineHealth {
    return EngineHealth{
        State:       e.state,
        Subscribers: len(e.subscribers),
        Routers:     len(e.routers),
        Publishers:  len(e.publishers),
    }
}
```

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                  message.Engine                                      │
│                                                                                      │
│  Subscribers              FanIn              Routers              FanOut             │
│  (by name)               (merge)           (by source)        (by destination)       │
│                                                                                      │
│  ┌──────────────┐                                                                    │
│  │ KafkaSub     │───┐                                                                │
│  │ name: kafka  │   │       ┌─────────┐                        ┌─────────────┐       │
│  └──────────────┘   │       │         │      ┌─────────┐       │             │       │
│                     ├──────►│ Routing │─────►│ Router  │──────►│  Routing    │       │
│  ┌──────────────┐   │       │ FanIn   │      │ source: │       │  FanOut     │       │
│  │ TickerSub    │───┤       │         │      │ orders  │       │             │       │
│  │ name: timer  │   │       │         │      └─────────┘       │ ┌─────────┐ │       │
│  └──────────────┘   │       │         │                        │ │kafka:// │─┼──────►│ KafkaPub
│                     │       │         │      ┌─────────┐       │ └─────────┘ │       │
│  ┌──────────────┐   │       │         │─────►│ Router  │──────►│             │       │
│  │ Loop Input   │───┘       │         │      │ source: │       │ ┌─────────┐ │       │
│  │ (internal)   │◄──────────│         │      │ payments│       │ │gopipe://│─┼───────┘
│  └──────────────┘           └─────────┘      └─────────┘       │ └─────────┘ │       │
│        ▲                                                       │             │       │
│        │                                                       └─────────────┘       │
│        └─────────────────────────────────────────────────────────────────────────────┘
│                                    Internal Loop
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## Implementation Order

```
1. RoutingFanIn ────────────────────────┐
                                        │
2. RoutingFanOut ───────────────────────┼──► 3. Engine Core
                                        │
                                        └──► 4. Registration APIs
                                                      │
                                                      ▼
                                              5. Lifecycle Management
```

**Recommended PR Sequence:**
1. **PR 1:** RoutingFanIn
2. **PR 2:** Engine FanOut (uses channel/routing_fanout.go)
3. **PR 3:** Engine Core + Registration + Lifecycle

## Usage Example

```go
func main() {
    ctx := context.Background()

    // Create engine
    engine := message.NewEngine(message.EngineConfig{
        LoopBufferSize:  1000,
        ShutdownTimeout: 30 * time.Second,
    })

    // === Subscribers ===
    kafkaSub := kafka.NewSubscriber(kafka.Config{
        Brokers: []string{"localhost:9092"},
        Topics:  []string{"orders", "payments"},
    })
    engine.AddSubscriber("kafka", kafkaSub)

    tickerSub := message.NewTickerSubscriber(time.Minute, func(ctx context.Context, t time.Time) ([]*message.Message, error) {
        return []*message.Message{
            message.MustNew(TimeoutCheck{}, message.Attributes{
                message.AttrType: "order.timeout.check",
            }),
        }, nil
    })
    engine.AddSubscriber("scheduler", tickerSub)

    // === Routers ===
    ordersRouter := message.NewRouter()
    ordersRouter.AddHandler("order.created", handleOrderCreated)
    ordersRouter.AddHandler("order.timeout.check", handleTimeoutCheck)
    engine.AddRouter("order", ordersRouter)

    // === Publishers ===
    kafkaPub := kafka.NewPublisher(kafka.Config{Brokers: []string{"localhost:9092"}})
    engine.AddPublisher("kafka://", kafkaPub)

    // === Start ===
    if err := engine.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Wait for shutdown signal
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    engine.Stop(context.Background())
}
```

## Validation Checklist

Before marking Layer 3 complete:

- [ ] RoutingFanIn merges multiple inputs
- [ ] RoutingFanOut routes by destination pattern
- [ ] Engine wires components correctly
- [ ] Internal loop via gopipe:// works
- [ ] SourceMatcher selects correct router
- [ ] Lifecycle: Start/Stop/Health work
- [ ] Error callbacks invoked appropriately
- [ ] Simple mode (without Engine) still works
- [ ] All tests pass
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0029: Message Engine](../adr/0029-message-engine.md)
- [Feature 17: Message Engine](../features/17-message-engine.md)
- [Layer 2: Routing Infrastructure](layer-2-routing-infrastructure.md)
