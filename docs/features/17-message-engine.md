# Feature 17: Message Engine

**Package:** `message`
**Status:** Proposed
**Priority:** High (orchestration layer for CloudEvents messaging)

**Related ADRs:**
- [ADR 0029](../adr/0029-message-engine.md) - Message Engine
- [ADR 0027](../adr/0027-fan-out-pattern.md) - Fan-Out Pattern (RoutingFanOut)
- [ADR 0028](../adr/0028-generator-source-patterns.md) - Subscriber Patterns
- [ADR 0023](../adr/0023-internal-message-loop.md) - Internal Message Loop

## Summary

The Message Engine is a top-level orchestration component that declaratively wires together subscribers, routers, and publishers. It handles internal feedback loops automatically via destination-based routing and provides a clean separation between orchestration and processing concerns.

**Key Benefits:**
- Declarative registration of components (subscribers, routers, publishers)
- Automatic internal loop handling via `gopipe://` destination
- Clear separation: Engine orchestrates, lower components process
- Simple mode available for basic use cases (Subscriber → Router → Publisher chain)

## Architecture Overview

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
│                     ├──────►│         │─────►│ Router  │──────►│             │       │
│  ┌──────────────┐   │       │         │      │ source: │       │  Routing    │       │
│  │ TickerSub    │───┤       │ FanIn   │      │ orders  │       │  FanOut     │       │
│  │ name: timer  │   │       │         │      └─────────┘       │             │       │
│  └──────────────┘   │       │         │                        │ ┌─────────┐ │       │
│                     │       │         │      ┌─────────┐       │ │kafka:// │─┼──────►│ KafkaPub
│  ┌──────────────┐   │       │         │─────►│ Router  │──────►│ └─────────┘ │       │
│  │ Internal     │───┘       │         │      │ source: │       │             │       │
│  │ Loop Input   │◄──────────│         │      │ payments│       │ ┌─────────┐ │       │
│  └──────────────┘           └─────────┘      └─────────┘       │ │nats://  │─┼──────►│ NATSPub
│        ▲                                                       │ └─────────┘ │       │
│        │                                                       │             │       │
│        │                                                       │ ┌─────────┐ │       │
│        └───────────────────────────────────────────────────────┤ │gopipe://│ │       │
│                            Internal Loop                       │ └─────────┘ │       │
│                                                                └─────────────┘       │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## Implementation

### Core Types

```go
// Engine orchestrates message flow between subscribers, routers, and publishers
type Engine struct {
    subscribers map[string]Subscriber[*Message]  // name → subscriber
    routers     map[string]*Router               // source → router
    publishers  map[string]Publisher             // destination pattern → publisher

    fanIn       *RoutingFanIn                    // merges subscriber outputs
    fanOut      *RoutingFanOut                   // routes to publishers/loops

    loopChan    chan *Message                    // internal loop channel
    config      EngineConfig
}

// EngineConfig configures engine behavior
type EngineConfig struct {
    // Error handling
    OnSubscriberError func(name string, err error)
    OnRouterError     func(source string, msg *Message, err error)
    OnPublisherError  func(dest string, msg *Message, err error)
    OnLoopOverflow    func(msg *Message)

    // Dead letter handling
    DeadLetterDest    string  // destination for unroutable messages

    // Graceful shutdown
    ShutdownTimeout   time.Duration
    DrainTimeout      time.Duration
}

// LoopConfig configures internal loop behavior
type LoopConfig struct {
    BufferSize   int                   // loop channel buffer
    DropOnFull   bool                  // drop vs block when full
    OnDrop       func(msg *Message)    // called when message dropped
}
```

### Registration APIs

```go
// NewEngine creates a new message engine
func NewEngine() *Engine

// AddSubscriber registers a subscriber with a logical name
func (e *Engine) AddSubscriber(name string, sub Subscriber[*Message]) error

// AddRouter registers a router for a source grouping
func (e *Engine) AddRouter(source string, router *Router) error

// AddPublisher registers a publisher for a destination pattern
func (e *Engine) AddPublisher(destinationPattern string, pub Publisher) error

// SetSourceMatcher customizes how messages are matched to routers
func (e *Engine) SetSourceMatcher(matcher SourceMatcher)

// SetLoopConfig configures internal loop behavior
func (e *Engine) SetLoopConfig(config LoopConfig)
```

### RoutingFanOut

```go
// RoutingFanOut routes messages to different channels based on a routing function
type RoutingFanOut struct {
    router    RoutingFunc
    outputs   map[string]chan<- *Message
    config    RoutingFanOutConfig
}

// RoutingFunc determines the destination key for a message
type RoutingFunc func(msg *Message) string

// RoutingFanOutConfig configures routing behavior
type RoutingFanOutConfig struct {
    DefaultOutput    string                         // fallback if no match
    OnNoMatch        func(msg *Message)             // called when no route matches
    OnSlowConsumer   func(dest string, msg *Message, elapsed time.Duration)
    BufferPerOutput  int                            // per-output buffer size
}

func NewRoutingFanOut(router RoutingFunc, config RoutingFanOutConfig) *RoutingFanOut
func (f *RoutingFanOut) AddOutput(pattern string, out chan<- *Message)
func (f *RoutingFanOut) Start(ctx context.Context, in <-chan *Message)
```

### RoutingFanIn

```go
// RoutingFanIn merges multiple input channels into one output
type RoutingFanIn struct {
    inputs  map[string]<-chan *Message
    output  chan *Message
    config  RoutingFanInConfig
}

type RoutingFanInConfig struct {
    BufferSize     int
    PreserveOrder  bool  // false = concurrent merge, true = round-robin
}

func NewRoutingFanIn(config RoutingFanInConfig) *RoutingFanIn
func (f *RoutingFanIn) AddInput(name string, in <-chan *Message)
func (f *RoutingFanIn) Output() <-chan *Message
func (f *RoutingFanIn) Start(ctx context.Context)
```

### Lifecycle

```go
// Engine states
type EngineState int

const (
    EngineStateCreated EngineState = iota
    EngineStateStarting
    EngineStateRunning
    EngineStateStopping
    EngineStateStopped
)

// Start begins message processing
func (e *Engine) Start(ctx context.Context) error

// Stop gracefully shuts down the engine
func (e *Engine) Stop(ctx context.Context) error

// Health returns current engine health
func (e *Engine) Health() EngineHealth

// Metrics returns engine metrics
func (e *Engine) Metrics() EngineMetrics
```

## Implementation Steps

### Step 17.1: RoutingFanIn

1. Create `RoutingFanIn` struct with configuration
2. Implement `AddInput` for dynamic input registration
3. Implement concurrent merge (default) and round-robin modes
4. Add context cancellation support

**Files:**
- `message/fanin.go` (new)
- `message/fanin_test.go` (new)

### Step 17.2: RoutingFanOut

1. Create `RoutingFanOut` struct with configuration
2. Implement destination-based routing function
3. Add slow consumer detection and callback
4. Support pattern matching for destinations

**Files:**
- `message/fanout.go` (new)
- `message/fanout_test.go` (new)

### Step 17.3: Engine Core

1. Create `Engine` struct with registration maps
2. Implement `AddSubscriber`, `AddRouter`, `AddPublisher`
3. Implement `SetSourceMatcher` with default type-prefix strategy
4. Wire FanIn and FanOut during `Start`

**Files:**
- `message/engine.go` (new)
- `message/engine_test.go` (new)

### Step 17.4: Internal Loop Integration

1. Create internal loop channel
2. Register `gopipe://` pattern with FanOut → loop channel
3. Connect loop channel to FanIn as additional input
4. Add `SetLoopConfig` for buffer/drop configuration

**Files:**
- `message/engine.go` (modify)
- `message/engine_loop_test.go` (new)

### Step 17.5: Lifecycle Management

1. Implement `Start` with proper initialization order
2. Implement `Stop` with graceful drain
3. Add state tracking and transitions
4. Implement `Health` and `Metrics`

**Files:**
- `message/engine.go` (modify)
- `message/engine_lifecycle_test.go` (new)

### Step 17.6: Error Handling

1. Implement error callbacks for subscribers, routers, publishers
2. Add dead letter destination support
3. Handle loop overflow scenarios
4. Add circuit breaker integration (optional)

**Files:**
- `message/engine_errors.go` (new)
- `message/engine_errors_test.go` (new)

## Usage Examples

### Full Engine Setup

```go
func main() {
    ctx := context.Background()

    // Create engine
    engine := message.NewEngine()

    // === Subscribers ===

    kafkaSub := kafka.NewSubscriber(kafka.SubscriberConfig{
        Brokers: []string{"localhost:9092"},
        Topics:  []string{"orders", "payments"},
        Group:   "order-service",
    })
    engine.AddSubscriber("kafka", kafkaSub)

    tickerSub := NewTickerSubscriber(time.Minute, func(ctx context.Context, t time.Time) ([]*Message, error) {
        return []*Message{
            MustNew(TimeoutCheck{}, WithType("order.timeout.check")),
        }, nil
    }, SubscriberConfig{})
    engine.AddSubscriber("scheduler", tickerSub)

    // === Routers ===

    ordersRouter := message.NewRouter()
    ordersRouter.AddHandler("order.created", orderCreatedHandler)
    ordersRouter.AddHandler("order.timeout.check", timeoutCheckHandler)
    engine.AddRouter("order", ordersRouter)

    paymentsRouter := message.NewRouter()
    paymentsRouter.AddHandler("payment.completed", paymentCompletedHandler)
    engine.AddRouter("payment", paymentsRouter)

    // Source matcher: extract from event type
    engine.SetSourceMatcher(func(msg *Message) string {
        typ := msg.Type()
        if idx := strings.Index(typ, "."); idx != -1 {
            return typ[:idx]  // "order.created" → "order"
        }
        return "default"
    })

    // === Publishers ===

    kafkaPub := kafka.NewPublisher(kafka.PublisherConfig{
        Brokers: []string{"localhost:9092"},
    })
    engine.AddPublisher("kafka://", kafkaPub)

    httpPub := http.NewPublisher(http.PublisherConfig{})
    engine.AddPublisher("http://", httpPub)
    engine.AddPublisher("https://", httpPub)

    // Internal loops are automatic (gopipe://)

    // === Start ===

    if err := engine.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Wait for shutdown
    <-ctx.Done()
    engine.Stop(context.Background())
}
```

### Simple Mode (Without Engine)

```go
// For simple use cases, components can be chained directly
sub := kafka.NewSubscriber(config)
router := message.NewRouter()
router.AddHandler("orders", handler)
pub := kafka.NewPublisher(config)

// Direct chaining - no Engine needed
ctx := context.Background()
msgs := sub.Subscribe(ctx)
processed := router.Start(ctx, msgs)
pub.PublishAll(ctx, processed)
```

### Handler Producing Multi-Destination Messages

```go
func orderCreatedHandler(ctx context.Context, msg *Message) ([]*Message, error) {
    order := msg.Data.(Order)

    return []*Message{
        // Internal: trigger inventory check (loops back)
        MustNew(InventoryCheck{OrderID: order.ID},
            WithType("inventory.check"),
            WithDestination("gopipe://inventory"),
        ),
        // External: notify analytics (goes to Kafka)
        MustNew(OrderAnalytics{...},
            WithType("analytics.order"),
            WithDestination("kafka://analytics"),
        ),
        // External: webhook (goes to HTTP)
        MustNew(WebhookPayload{...},
            WithType("webhook.order"),
            WithDestination("https://partner.com/webhook"),
        ),
    }, nil
}
```

## Component Comparison

| Component | Routes To | Criteria | Use Case |
|-----------|-----------|----------|----------|
| **Broadcast** | ALL outputs | None | CQRS projections, audit |
| **Router** | ONE handler | Event type/topic | Handler dispatch |
| **RoutingFanOut** | ONE output | Destination function | Publisher/loop routing |

## When to Use Engine vs Simple Mode

| Use Case | Recommendation |
|----------|----------------|
| Single subscriber, single publisher | Simple chaining |
| Multiple subscribers | Engine |
| Internal feedback loops | Engine |
| Multiple destination types | Engine |
| Complex routing by source | Engine |
| Microservice with clear flow | Simple chaining |
| Event-driven orchestration | Engine |

## Files Changed

### New Files

- `message/engine.go` - Engine struct and registration methods
- `message/engine_test.go` - Engine unit tests
- `message/fanin.go` - RoutingFanIn implementation
- `message/fanin_test.go` - FanIn tests
- `message/fanout.go` - RoutingFanOut implementation
- `message/fanout_test.go` - FanOut tests
- `message/engine_errors.go` - Error handling utilities
- `message/engine_lifecycle_test.go` - Lifecycle tests

### Modified Files

- `message/router.go` - Ensure Router doesn't inspect destination
- `channel/fanout.go` - Add RoutingFanOut (or keep in message/)

## Dependencies

This feature depends on:
- [Feature 12: Internal Message Routing](12-internal-message-routing.md) - Router component
- [Feature 13: Internal Message Loop](13-internal-message-loop.md) - Loop concepts
- [Feature 16: Core Pipe Refactoring](16-core-pipe-refactoring.md) - Subscriber interface

This feature enables:
- Complex event-driven architectures
- Multi-protocol publishing
- Automatic internal feedback loops

## Acceptance Criteria

### Core Engine
1. [ ] `Engine` struct with subscriber/router/publisher registration
2. [ ] `AddSubscriber(name, sub)` registers subscribers by logical name
3. [ ] `AddRouter(source, router)` registers routers by source grouping
4. [ ] `AddPublisher(pattern, pub)` registers publishers by destination pattern
5. [ ] `SetSourceMatcher(fn)` configures message-to-router matching

### FanIn/FanOut
6. [ ] `RoutingFanIn` merges multiple subscriber outputs
7. [ ] `RoutingFanOut` routes by destination to ONE output
8. [ ] Slow consumer detection and callback
9. [ ] Buffer configuration per output

### Internal Loops
10. [ ] `gopipe://` destinations automatically loop back
11. [ ] `SetLoopConfig` configures loop buffer and drop behavior
12. [ ] Loop channel connected to FanIn

### Lifecycle
13. [ ] `Start(ctx)` initializes and starts all components
14. [ ] `Stop(ctx)` gracefully drains and stops
15. [ ] `Health()` returns component health status
16. [ ] `Metrics()` returns processing metrics

### Error Handling
17. [ ] Error callbacks for subscribers, routers, publishers
18. [ ] Dead letter destination for unroutable messages
19. [ ] Loop overflow handling

### Simple Mode
20. [ ] Components work independently without Engine
21. [ ] Subscriber → Router → Publisher chain supported

## Related Features

- [Feature 12: Internal Message Routing](12-internal-message-routing.md) - Router component
- [Feature 13: Internal Message Loop](13-internal-message-loop.md) - Loop patterns
- [Feature 16: Core Pipe Refactoring](16-core-pipe-refactoring.md) - Subscriber interface
