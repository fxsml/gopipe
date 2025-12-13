# ADR 0028: Generator and Source Patterns

**Date:** 2025-12-13
**Status:** Proposed
**Related:** ADR 0026 (Pipe Simplification), ADR 0023 (Internal Loop)

## Context

The current `Generator` implementation works but raises questions:

1. What is the purpose of Generator in a messaging system?
2. How does it relate to Subscriber?
3. Should there be a unified "Source" abstraction?
4. What use cases does Generator serve?

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

### Generator vs Subscriber Analysis

| Aspect | Generator | Subscriber |
|--------|-----------|------------|
| **Source** | Internal logic | External system |
| **Trigger** | Time/logic-driven | Event-driven |
| **Examples** | Timer, poller, calculator | Kafka, NATS, HTTP |
| **Rate Control** | Generator controls | External controls |
| **Backpressure** | Can throttle | Must handle or drop |

**Key Insight:** Both are **Sources** - they produce messages without consuming from upstream.

```
Source (no input) → Processor → Processor → Sink

Generator (internal): Timer → Process → Publish
Subscriber (external): Kafka → Process → Publish
```

## Decision

### 1. Unified Source Interface

Define a common `Source` interface that both Generator and Subscriber implement:

```go
// Source produces a stream of values without input
type Source[Out any] interface {
    // Start begins producing values until context cancellation
    Start(ctx context.Context) <-chan Out
}

// Generator is a Source that produces values from internal logic
type Generator[Out any] Source[Out]

// Subscriber is a Source that receives values from external systems
type Subscriber[Out any] Source[Out]
```

**Why not use Pipe?**

`Pipe[In, Out]` requires an input channel. Sources have no input.

```go
// Pipe - has input
type Pipe[In, Out any] interface {
    Start(ctx context.Context, in <-chan In) <-chan Out
}

// Source - no input
type Source[Out any] interface {
    Start(ctx context.Context) <-chan Out
}
```

### 2. Generator Config (Not Options)

Following ADR 0026, use config struct instead of functional options:

```go
type GeneratorConfig struct {
    // Rate limiting
    Interval    time.Duration // Min time between generations, 0 = as fast as possible
    MaxPerBatch int           // Max items per generation call, 0 = unlimited

    // Error handling
    OnError      func(err error) // Called on generation error
    RetryOnError bool            // Retry on error, default: false
    MaxRetries   int             // Max consecutive retries, 0 = unlimited

    // Lifecycle
    StopOnError bool // Stop generator on first error, default: false

    // Output
    Buffer int // Output channel buffer, default: 0
}

var DefaultGeneratorConfig = GeneratorConfig{
    Interval:     0,
    MaxPerBatch:  0,
    RetryOnError: false,
    StopOnError:  false,
    Buffer:       0,
}
```

### 3. Generator Factory Functions

Provide factory functions for common patterns:

```go
// NewGenerator creates a generator from a function
func NewGenerator[Out any](
    generate func(context.Context) ([]Out, error),
    config GeneratorConfig,
) Source[Out]

// NewTickerGenerator generates on time intervals
func NewTickerGenerator[Out any](
    interval time.Duration,
    generate func(ctx context.Context, tick time.Time) ([]Out, error),
    config GeneratorConfig,
) Source[Out]

// NewPollingGenerator polls an external source
func NewPollingGenerator[Out any](
    interval time.Duration,
    poll func(ctx context.Context) ([]Out, error),
    config GeneratorConfig,
) Source[Out]
```

### 4. Generator Use Cases

#### Use Case 1: Time-Triggered Events (Heartbeats)

```go
// Send heartbeat every 30 seconds
heartbeat := NewTickerGenerator(
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
    GeneratorConfig{},
)
```

#### Use Case 2: Scheduled Tasks (Cron-like)

```go
// Generate daily report events at midnight
scheduler := NewTickerGenerator(
    24*time.Hour,
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
    GeneratorConfig{},
)
```

#### Use Case 3: Polling External API

```go
// Poll weather API every 5 minutes
weatherPoller := NewPollingGenerator(
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
    GeneratorConfig{
        RetryOnError: true,
        MaxRetries:   3,
    },
)
```

#### Use Case 4: File Watching

```go
// Emit events when files change
watcher := NewGenerator(
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
    GeneratorConfig{},
)
```

#### Use Case 5: Synthetic Events (Derived)

```go
// Generate aggregate statistics every minute
statsGenerator := NewTickerGenerator(
    time.Minute,
    func(ctx context.Context, tick time.Time) ([]*Message, error) {
        stats := collectMetrics()
        return []*Message{
            NewMessage(
                WithType("metrics.collected"),
                WithSource("gopipe://metrics-collector"),
                WithData(stats),
            ),
        }, nil
    },
    GeneratorConfig{},
)
```

### 5. Is Generator a Subscriber?

**Answer: No, but they share the Source interface.**

| Characteristic | Generator | Subscriber |
|---------------|-----------|------------|
| **Input source** | Internal logic | External broker |
| **Who triggers?** | Generator (push) | Broker (push to subscriber) |
| **Rate control** | Config.Interval | Broker delivery rate |
| **Acknowledgment** | Not needed | Required for reliability |
| **Replay** | N/A (stateless) | From offset/sequence |

**However**, both:
- Implement `Source[Out]` interface
- Produce `<-chan Out`
- Can be used interchangeably where a Source is needed

```go
// Router accepts any Source
func (r *Router) AddSource(name string, source Source[*Message])

// Can add either
router.AddSource("heartbeats", heartbeatGenerator)
router.AddSource("orders", kafkaSubscriber)
```

### 6. Subscriber as Specialized Source

```go
// Subscriber extends Source with messaging-specific methods
type Subscriber interface {
    Source[*Message]

    // Messaging-specific
    Acknowledge(ctx context.Context, msg *Message) error
    Seek(ctx context.Context, offset int64) error
}
```

**Why separate interface?**

- Generator doesn't need Acknowledge (no external state)
- Generator doesn't need Seek (stateless)
- Subscriber needs these for reliability

### 7. Message Generator (CloudEvents)

For CloudEvents compliance, provide message-specific generator:

```go
// MessageGenerator produces CloudEvents-compliant messages
type MessageGenerator interface {
    Source[*Message]
}

// NewMessageGenerator enforces CloudEvents compliance
func NewMessageGenerator(
    eventType string,           // Required: CloudEvents type
    source string,              // Required: CloudEvents source
    generate func(ctx context.Context) (any, error),  // Generate data
    config GeneratorConfig,
) MessageGenerator
```

**Usage:**

```go
// All generated messages have proper CloudEvents attributes
gen := NewMessageGenerator(
    "order.timeout.check",
    "gopipe://timeout-checker",
    func(ctx context.Context) (any, error) {
        return TimeoutCheckRequest{Threshold: time.Hour}, nil
    },
    GeneratorConfig{Interval: time.Minute},
)
```

### 8. Integration with Internal Loop (ADR 0023)

Generators can publish to internal topics:

```go
// Generator publishes to internal loop
heartbeat := NewMessageGenerator(
    "system.heartbeat",
    "gopipe://service",
    generateHeartbeat,
    GeneratorConfig{Interval: 30 * time.Second},
)

// Route generator output to internal topic
loop.AddSource("heartbeat-gen", heartbeat)
loop.Route("system.heartbeat", "gopipe://monitoring", monitoringHandler)
```

## Rationale

### Why Separate Source from Pipe?

1. **Semantic clarity**: Sources have no input, Pipes transform
2. **Type safety**: No `struct{}` hack needed
3. **Config simplicity**: No unused input-related options

### Why Not Make Generator a Subscriber?

1. **Different semantics**: Generator is push, Subscriber is pull
2. **Different lifecycle**: Generator controls rate, Subscriber follows broker
3. **Different reliability**: Generator is stateless, Subscriber needs acks

### Why Config Struct for Generator?

1. **Consistency**: Matches ADR 0026 pattern
2. **Simplicity**: No generic parameters in config
3. **Discoverability**: All options visible in struct

## Consequences

### Positive

- Unified `Source` interface for Generators and Subscribers
- Config-based Generator matches simplified Pipe (ADR 0026)
- Factory functions for common patterns (ticker, polling)
- CloudEvents-compliant MessageGenerator
- Clear distinction from Subscriber

### Negative

- New `Source` interface to learn
- Migration from current Generator API
- Two interfaces (Source, Subscriber) where one might seem sufficient

### Migration

```go
// Old
gen := NewGenerator(func(ctx context.Context) ([]int, error) {
    return []int{1, 2, 3}, nil
}, WithConcurrency[struct{}, int](1))

// New
gen := NewGenerator(func(ctx context.Context) ([]int, error) {
    return []int{1, 2, 3}, nil
}, GeneratorConfig{})

// Or use ticker for time-based
gen := NewTickerGenerator(time.Second, func(ctx context.Context, t time.Time) ([]int, error) {
    return []int{1, 2, 3}, nil
}, GeneratorConfig{})
```

## Links

- [ADR 0026: Pipe and Processor Simplification](0026-pipe-processor-simplification.md)
- [ADR 0023: Internal Message Loop](0023-internal-message-loop.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
