# ADR 0029: Message Engine

**Date:** 2025-12-15
**Status:** Proposed
**Related:** ADR 0023 (Internal Loop), ADR 0027 (Fan-Out), ADR 0028 (Subscriber)

## Context

The current architecture has separate components (Subscriber, Router, Publisher) that must be manually wired together. For complex scenarios with multiple subscribers, routers, and internal feedback loops, this becomes error-prone and verbose.

We need:
1. A top-level orchestration component (`message.Engine`)
2. Declarative registration of subscribers, routers, and publishers
3. Automatic internal loop handling via destination-based routing
4. Simple mode for basic use cases without the Engine

### Current State: Manual Wiring

```go
// Complex - manual wiring required
sub := kafka.NewSubscriber(...)
router := message.NewRouter()
router.AddHandler("orders", orderHandler)
pub := kafka.NewPublisher(...)

// Manual loop handling
msgs := sub.Subscribe(ctx)
processed := router.Start(ctx, msgs)
for msg := range processed {
    if strings.HasPrefix(msg.Destination(), "gopipe://") {
        // Manual loop back... complex!
    } else {
        pub.Publish(ctx, msg)
    }
}
```

### Desired State: Declarative Engine

```go
engine := message.NewEngine()

// Register subscribers (produce to logical destinations)
engine.AddSubscriber("external", kafkaSub)
engine.AddSubscriber("timers", tickerSub)

// Register routers (handle by logical source grouping)
engine.AddRouter("orders", ordersRouter)
engine.AddRouter("payments", paymentsRouter)

// Register publishers (consume from destination patterns)
engine.AddPublisher("kafka://", kafkaPub)
engine.AddPublisher("nats://", natsPub)

// Internal loops are automatic for gopipe:// destinations
engine.Start(ctx)
```

## Decision

### 1. Engine as Top-Level Orchestrator

```go
// Engine orchestrates message flow between subscribers, routers, and publishers
type Engine struct {
    subscribers map[string]Subscriber[*Message]  // name → subscriber
    routers     map[string]*Router               // source → router
    publishers  map[string]Publisher             // destination pattern → publisher

    fanIn       *RoutingFanIn                    // merges subscriber outputs
    fanOut      *RoutingFanOut                   // routes to publishers/loops

    loopChan    chan *Message                    // internal loop channel
}

func NewEngine() *Engine
func (e *Engine) Start(ctx context.Context) error
func (e *Engine) Stop(ctx context.Context) error
```

### 2. Architecture Overview

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

### 3. Registration APIs

#### Subscribers (by name)

```go
// AddSubscriber registers a subscriber with a logical name
// All messages from this subscriber are merged into the FanIn
func (e *Engine) AddSubscriber(name string, sub Subscriber[*Message]) error

// Example
engine.AddSubscriber("orders-kafka", kafkaSub)
engine.AddSubscriber("heartbeat", tickerSub)
```

#### Routers (by source grouping)

```go
// AddRouter registers a router for a source grouping
// Messages are routed to this router based on source matching
func (e *Engine) AddRouter(source string, router *Router) error

// SourceMatcher determines which messages go to which router
type SourceMatcher func(msg *Message) string

// SetSourceMatcher customizes how messages are matched to routers
// Default: uses message type prefix (e.g., "order.created" → "order")
func (e *Engine) SetSourceMatcher(matcher SourceMatcher)

// Example
engine.AddRouter("orders", ordersRouter)    // handles order.* types
engine.AddRouter("payments", paymentsRouter) // handles payment.* types
```

**Source Matching Strategies:**

| Strategy | Description | Example |
|----------|-------------|---------|
| Type prefix | Extract prefix from event type | `order.created` → `orders` |
| Topic | Use message topic attribute | `topic: orders` → `orders` |
| CE Source | Use CloudEvents source attribute | `source: /orders/api` → `orders` |
| Custom | User-defined function | Any logic |

#### Publishers (by destination pattern)

```go
// AddPublisher registers a publisher for a destination pattern
// Messages with matching destinations are routed to this publisher
func (e *Engine) AddPublisher(destinationPattern string, pub Publisher) error

// Example
engine.AddPublisher("kafka://", kafkaPub)      // handles kafka:// destinations
engine.AddPublisher("nats://", natsPub)        // handles nats:// destinations
engine.AddPublisher("http://", httpPub)        // handles http:// destinations
engine.AddPublisher("https://", httpsPub)      // handles https:// destinations
```

#### Internal Loops (automatic)

```go
// Internal loops are automatic - messages with gopipe:// destination
// are routed back to the FanIn

// Optionally configure loop behavior
engine.SetLoopConfig(LoopConfig{
    BufferSize:   1000,            // loop channel buffer
    DropOnFull:   false,           // drop vs block when full
    OnDrop:       func(msg *Message) { /* log dropped */ },
})
```

### 4. Routing FanOut

Enhance FanOut to support destination-based routing:

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

// NewRoutingFanOut creates a routing fan-out
func NewRoutingFanOut(
    router RoutingFunc,
    config RoutingFanOutConfig,
) *RoutingFanOut

// AddOutput registers an output channel for a destination pattern
func (f *RoutingFanOut) AddOutput(pattern string, out chan<- *Message)

// Start begins routing from input to outputs
func (f *RoutingFanOut) Start(ctx context.Context, in <-chan *Message)
```

**Routing by Destination:**

```go
// Default routing function - uses destination attribute
func defaultDestinationRouter(msg *Message) string {
    dest := msg.Destination()
    if dest == "" {
        return "default"
    }
    // Extract scheme: "kafka://topic" → "kafka://"
    if idx := strings.Index(dest, "://"); idx != -1 {
        return dest[:idx+3]
    }
    return dest
}
```

### 5. Routing FanIn

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

### 6. Simple Mode (Without Engine)

For simple use cases, components can be chained directly:

```go
// Simple: Subscriber → Router → Publisher
sub := kafka.NewSubscriber(config)
router := message.NewRouter()
router.AddHandler("orders", handler)
pub := kafka.NewPublisher(config)

// Direct chaining
ctx := context.Background()
msgs := sub.Subscribe(ctx)
processed := router.Start(ctx, msgs)
pub.PublishAll(ctx, processed)
```

**When to use Engine vs Simple:**

| Use Case | Recommendation |
|----------|----------------|
| Single subscriber, single publisher | Simple chaining |
| Multiple subscribers | Engine |
| Internal feedback loops | Engine |
| Multiple destination types | Engine |
| Complex routing by source | Engine |
| Microservice with clear flow | Simple chaining |
| Event-driven orchestration | Engine |

### 7. Source/Destination at Engine Level Only

**Important:** Source and destination concepts for routing are ONLY used at the Engine level. Lower-level components (Router, Handler, Pipe) should NOT inspect these for routing decisions.

```go
// Router - does NOT route by source/destination
// Uses topic or type for handler dispatch
type Router struct {
    handlers map[string]Handler  // topic/type → handler
}

// Engine - DOES route by source/destination
// Uses source for router selection, destination for publisher selection
type Engine struct {
    routers    map[string]*Router   // source → router
    publishers map[string]Publisher // destination → publisher
}
```

### 8. Complete Example

```go
func main() {
    ctx := context.Background()

    // Create engine
    engine := message.NewEngine()

    // === Subscribers ===

    // Kafka subscriber for external orders
    kafkaSub := kafka.NewSubscriber(kafka.SubscriberConfig{
        Brokers: []string{"localhost:9092"},
        Topics:  []string{"orders", "payments"},
        Group:   "order-service",
    })
    engine.AddSubscriber("kafka", kafkaSub)

    // Ticker for scheduled tasks
    tickerSub := NewTickerSubscriber(time.Minute, func(ctx context.Context, t time.Time) ([]*Message, error) {
        return []*Message{
            MustNew(TimeoutCheck{}, WithType("order.timeout.check")),
        }, nil
    }, SubscriberConfig{})
    engine.AddSubscriber("scheduler", tickerSub)

    // === Routers ===

    // Orders router
    ordersRouter := message.NewRouter()
    ordersRouter.AddHandler("order.created", orderCreatedHandler)
    ordersRouter.AddHandler("order.timeout.check", timeoutCheckHandler)
    engine.AddRouter("order", ordersRouter)

    // Payments router
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

    // Kafka publisher for external events
    kafkaPub := kafka.NewPublisher(kafka.PublisherConfig{
        Brokers: []string{"localhost:9092"},
    })
    engine.AddPublisher("kafka://", kafkaPub)

    // HTTP publisher for webhooks
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

// Handler that produces internal and external messages
func orderCreatedHandler(ctx context.Context, msg *Message) ([]*Message, error) {
    order := msg.Data.(Order)

    return []*Message{
        // Internal: trigger inventory check
        MustNew(InventoryCheck{OrderID: order.ID},
            WithType("inventory.check"),
            WithDestination("gopipe://inventory"),  // loops back
        ),
        // External: notify analytics
        MustNew(OrderAnalytics{...},
            WithType("analytics.order"),
            WithDestination("kafka://analytics"),  // goes to Kafka
        ),
        // External: webhook
        MustNew(WebhookPayload{...},
            WithType("webhook.order"),
            WithDestination("https://partner.com/webhook"),  // goes to HTTP
        ),
    }, nil
}
```

### 9. Engine Lifecycle

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

// Lifecycle methods
func (e *Engine) Start(ctx context.Context) error {
    // 1. Validate configuration
    // 2. Start all subscribers
    // 3. Start FanIn
    // 4. Start all routers
    // 5. Start FanOut
    // 6. Start all publishers
    // 7. Connect loop channel
}

func (e *Engine) Stop(ctx context.Context) error {
    // 1. Stop accepting new messages
    // 2. Drain in-flight messages
    // 3. Stop publishers
    // 4. Stop routers
    // 5. Stop subscribers
}

// Health and metrics
func (e *Engine) Health() EngineHealth
func (e *Engine) Metrics() EngineMetrics
```

### 10. Error Handling

```go
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
```

## Rationale

### Why Engine?

1. **Declarative**: Register components, Engine handles wiring
2. **Internal loops**: Automatic `gopipe://` loop handling
3. **Separation**: Source for router selection, destination for publisher selection
4. **Scalability**: Easy to add subscribers, routers, publishers

### Why Source/Destination at Engine Level Only?

1. **Simplicity**: Lower components don't need routing logic
2. **Testability**: Router can be tested without destination awareness
3. **Flexibility**: Different engines can route differently
4. **Separation of concerns**: Engine orchestrates, components process

### Why RoutingFanOut Instead of Broadcast?

1. **Efficiency**: Message goes to ONE output, not all
2. **Destination-aware**: Routes based on message content
3. **Extensible**: Custom routing functions supported

## Consequences

### Positive

- Declarative registration simplifies complex setups
- Automatic internal loop handling
- Clear separation between orchestration and processing
- Simple mode remains available for basic use cases
- Destination-based routing enables multi-protocol publishing

### Negative

- Additional abstraction layer
- Engine adds complexity for simple use cases (but optional)
- Source matching strategy must be chosen/configured

### Migration

```go
// Old: Manual wiring with internal loop handling
sub := kafka.NewSubscriber(...)
router := message.NewRouter()
pub := kafka.NewPublisher(...)
// ... complex loop handling ...

// New: Declarative Engine
engine := message.NewEngine()
engine.AddSubscriber("kafka", sub)
engine.AddRouter("orders", router)
engine.AddPublisher("kafka://", pub)
engine.Start(ctx)
```

## Links

- [ADR 0023: Internal Message Loop](0023-internal-message-loop.md)
- [ADR 0027: Fan-Out Pattern](0027-fan-out-pattern.md)
- [ADR 0028: Subscriber Patterns](0028-generator-source-patterns.md)
- [Feature 17: Message Engine](../features/17-message-engine.md)
