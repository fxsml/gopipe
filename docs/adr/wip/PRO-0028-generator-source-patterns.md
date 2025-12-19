# ADR 0028: Subscriber Patterns

**Date:** 2025-12-13
**Status:** Proposed
**Related:** ADR 0026 (Pipe Simplification), ADR 0023 (Internal Loop)

## Context

The current `Generator` implementation works but raises questions:

1. What is the purpose of Generator in a messaging system?
2. How does it relate to Subscriber?
3. Should there be a unified abstraction?
4. What use cases does Generator serve?

### Naming Consideration

Initially, we considered a "Source" interface. However, `source` is already a **CloudEvents required attribute** identifying the event origin. Using "Source" as an interface name would cause confusion:

```go
// Confusing - "source" means two different things
msg.SetSource("gopipe://orders")  // CloudEvents attribute
router.AddSource("orders", sub)   // Interface type
```

**Decision:** Use `Subscriber` as the base type for all message producers. This aligns with messaging terminology where you "subscribe" to various sources of events.

### Current Implementation

```go
// Generator wraps a Processor with struct{} input
type generator[Out any] struct {
    proc Processor[struct{}, Out]
    opts []Option[struct{}, Out]
}

func (g *generator[Out]) Generate(ctx context.Context) <-chan Out {
    return startProcessor(ctx, channel.FromFunc(ctx, func() struct{} {
        return struct{}{}
    }), g.proc, g.opts)
}
```

**Issues:**
- `struct{}` input is an implementation detail that leaks into Options
- Option generics `[struct{}, Out]` are confusing
- No built-in support for common patterns (timers, polling)
- Relationship to Subscriber unclear

## Decision

### 1. Subscriber as Unified Base Interface

All message producers implement `Subscriber`:

```go
// Subscriber produces a stream of messages
type Subscriber[Out any] interface {
    // Subscribe begins producing values until context cancellation
    Subscribe(ctx context.Context) <-chan Out
}
```

**Why not use Pipe?**

`Pipe[In, Out]` requires an input channel. Subscribers have no input:

```go
// Pipe - has input
type Pipe[In, Out any] interface {
    Start(ctx context.Context, in <-chan In) <-chan Out
}

// Subscriber - no input
type Subscriber[Out any] interface {
    Subscribe(ctx context.Context) <-chan Out
}
```

### 2. Subscriber Type Hierarchy

```
Subscriber[Out any]
├── BrokerSubscriber      - External message brokers (Kafka, NATS, etc.)
├── TickerSubscriber      - Time-triggered events (heartbeats, scheduled)
├── PollingSubscriber     - Polls external APIs periodically
├── ChannelSubscriber     - Receives from Go channels
└── WatcherSubscriber     - File system, database changes
```

| Subscriber Type | Trigger | Use Case |
|-----------------|---------|----------|
| `BrokerSubscriber` | External events | Kafka, NATS, RabbitMQ messages |
| `TickerSubscriber` | Time intervals | Heartbeats, scheduled tasks |
| `PollingSubscriber` | Time + external | API polling, health checks |
| `ChannelSubscriber` | Go channel | Internal pipelines, testing |
| `WatcherSubscriber` | External changes | File changes, DB triggers |

### 3. SubscriberConfig (Replaces GeneratorConfig)

Following ADR 0026, use config struct instead of functional options:

```go
type SubscriberConfig struct {
    // Rate limiting (for ticker/polling types)
    Interval    time.Duration // Min time between events, 0 = as fast as possible
    MaxPerBatch int           // Max items per batch, 0 = unlimited

    // Error handling
    OnError      func(err error) // Called on error
    RetryOnError bool            // Retry on error, default: false
    MaxRetries   int             // Max consecutive retries, 0 = unlimited

    // Lifecycle
    StopOnError bool // Stop on first error, default: false

    // Output
    Buffer int // Output channel buffer, default: 0
}

var DefaultSubscriberConfig = SubscriberConfig{
    Interval:     0,
    MaxPerBatch:  0,
    RetryOnError: false,
    StopOnError:  false,
    Buffer:       0,
}
```

### 4. Subscriber Factory Functions

```go
// TickerSubscriber - time-triggered events
func NewTickerSubscriber[Out any](
    interval time.Duration,
    generate func(ctx context.Context, tick time.Time) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]

// PollingSubscriber - polls external source
func NewPollingSubscriber[Out any](
    interval time.Duration,
    poll func(ctx context.Context) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]

// ChannelSubscriber - wraps a Go channel
func NewChannelSubscriber[Out any](
    ch <-chan Out,
) Subscriber[Out]

// FuncSubscriber - custom logic (replaces NewGenerator)
func NewFuncSubscriber[Out any](
    generate func(ctx context.Context) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]
```

### 5. BrokerSubscriber Interface

External brokers need additional capabilities:

```go
// BrokerSubscriber extends Subscriber with messaging-specific methods
type BrokerSubscriber interface {
    Subscriber[*Message]

    // Messaging-specific
    Acknowledge(ctx context.Context, msg *Message) error
    Seek(ctx context.Context, offset int64) error

    // Optional
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
}
```

**Why separate interface?**

- `TickerSubscriber` doesn't need Acknowledge (no external state)
- `PollingSubscriber` doesn't need Seek (stateless)
- `BrokerSubscriber` needs these for reliability

### 6. Subscriber Use Cases

#### Use Case 1: Time-Triggered Events (Heartbeats)

```go
// Send heartbeat every 30 seconds
heartbeat := NewTickerSubscriber(
    30*time.Second,
    func(ctx context.Context, tick time.Time) ([]*Message, error) {
        return []*Message{
            NewMessage(
                WithType("system.heartbeat"),
                WithSource("gopipe://service-a"),
                WithData(HeartbeatData{Timestamp: tick, Status: "healthy"}),
            ),
        }, nil
    },
    SubscriberConfig{},
)
```

#### Use Case 2: Scheduled Tasks (Cron-like)

```go
// Generate daily report events
scheduler := NewTickerSubscriber(
    time.Hour,
    func(ctx context.Context, tick time.Time) ([]*Message, error) {
        if tick.Hour() != 0 { // Only at midnight
            return nil, nil
        }
        return []*Message{
            NewMessage(
                WithType("report.daily.generate"),
                WithSource("gopipe://scheduler"),
                WithData(ReportRequest{Date: tick.Truncate(24 * time.Hour)}),
            ),
        }, nil
    },
    SubscriberConfig{},
)
```

#### Use Case 3: Polling External API

```go
// Poll weather API every 5 minutes
weatherPoller := NewPollingSubscriber(
    5*time.Minute,
    func(ctx context.Context) ([]*Message, error) {
        resp, err := http.Get("https://api.weather.com/current")
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()

        var weather WeatherData
        if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
            return nil, err
        }

        return []*Message{
            NewMessage(
                WithType("weather.updated"),
                WithSource("https://api.weather.com"),
                WithData(weather),
            ),
        }, nil
    },
    SubscriberConfig{
        RetryOnError: true,
        MaxRetries:   3,
    },
)
```

#### Use Case 4: File Watching

```go
// Emit events when files change
watcher := NewFuncSubscriber(
    func(ctx context.Context) ([]*Message, error) {
        event, err := fsWatcher.Next(ctx)
        if err != nil {
            return nil, err
        }
        return []*Message{
            NewMessage(
                WithType("file." + string(event.Op)),
                WithSource("file://" + event.Path),
                WithData(event),
            ),
        }, nil
    },
    SubscriberConfig{},
)
```

#### Use Case 5: Kafka Broker

```go
// Subscribe to Kafka topic
kafkaSub := kafka.NewSubscriber(kafka.SubscriberConfig{
    Brokers: []string{"localhost:9092"},
    Topic:   "orders",
    Group:   "order-processor",
})

// Use like any other subscriber
msgs := kafkaSub.Subscribe(ctx)
for msg := range msgs {
    // Process message
    kafkaSub.Acknowledge(ctx, msg)
}
```

### 7. Subscriber Comparison Table

| Feature | TickerSubscriber | PollingSubscriber | BrokerSubscriber |
|---------|------------------|-------------------|------------------|
| **Trigger** | Time interval | Time + external | External events |
| **Rate control** | Config.Interval | Config.Interval | Broker controls |
| **Acknowledgment** | Not needed | Not needed | Required |
| **Replay/Seek** | N/A | N/A | Supported |
| **Backpressure** | Drop or block | Drop or block | Broker handles |
| **State** | Stateless | Stateless | Offset tracking |

### 8. Message-Aware Subscriber

For CloudEvents compliance, provide message-specific subscriber:

```go
// MessageSubscriber produces CloudEvents-compliant messages
func NewMessageSubscriber(
    eventType string,           // Required: CloudEvents type
    source string,              // Required: CloudEvents source
    generate func(ctx context.Context) (any, error),
    config SubscriberConfig,
) Subscriber[*Message]
```

**Usage:**

```go
// All messages have proper CloudEvents attributes
sub := NewMessageSubscriber(
    "order.timeout.check",
    "gopipe://timeout-checker",
    func(ctx context.Context) (any, error) {
        return TimeoutCheckRequest{Threshold: time.Hour}, nil
    },
    SubscriberConfig{Interval: time.Minute},
)
```

### 9. Integration with Router

```go
// Router accepts any Subscriber
func (r *Router) AddSubscriber(name string, sub Subscriber[*Message])

// Can add any subscriber type
router.AddSubscriber("heartbeats", heartbeatTicker)
router.AddSubscriber("orders", kafkaBrokerSub)
router.AddSubscriber("weather", weatherPoller)
```

### 10. Integration with Internal Loop (ADR 0023)

```go
// Ticker publishes to internal loop
heartbeat := NewTickerSubscriber(
    30*time.Second,
    generateHeartbeat,
    SubscriberConfig{},
)

// Route to internal topic
loop.AddSubscriber("heartbeat-ticker", heartbeat)
loop.Route("system.heartbeat", "gopipe://monitoring", monitoringHandler)
```

## Rationale

### Why Use Subscriber Instead of Source?

1. **Avoids CloudEvents confusion**: `source` is a CE attribute
2. **Messaging alignment**: "Subscribe" is standard messaging terminology
3. **Clear intent**: You subscribe to time, brokers, APIs, etc.
4. **Extensibility**: Easy to add new subscriber types

### Why Subscriber as Base Type?

1. **Unified interface**: All message producers share same contract
2. **Interchangeable**: Router accepts any Subscriber
3. **Type safety**: No `struct{}` hack needed
4. **Config simplicity**: No unused input-related options

### Why Specialized Subscriber Types?

1. **Clear purpose**: Name indicates what you're subscribing to
2. **Optimized interfaces**: BrokerSubscriber has Ack, others don't
3. **Documentation**: Easy to find right type for use case
4. **Testing**: Can mock specific subscriber types

## Consequences

### Positive

- Avoids `source` attribute naming conflict
- Clear subscriber type hierarchy
- Config-based matches ADR 0026 pattern
- Factory functions for common patterns
- CloudEvents-compliant MessageSubscriber
- BrokerSubscriber extends base for messaging needs

### Negative

- Rename from Generator to XxxSubscriber (breaking change)
- Multiple subscriber types to learn
- "Subscriber" may feel odd for time-based events

### Migration

```go
// Old
gen := NewGenerator(func(ctx context.Context) ([]int, error) {
    return []int{1, 2, 3}, nil
}, WithConcurrency[struct{}, int](1))

// New
sub := NewFuncSubscriber(func(ctx context.Context) ([]int, error) {
    return []int{1, 2, 3}, nil
}, SubscriberConfig{})

// Or use ticker for time-based
sub := NewTickerSubscriber(time.Second, func(ctx context.Context, t time.Time) ([]int, error) {
    return []int{1, 2, 3}, nil
}, SubscriberConfig{})
```

## Links

- [ADR 0026: Pipe and Processor Simplification](0026-pipe-processor-simplification.md)
- [ADR 0023: Internal Message Loop](0023-internal-message-loop.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
