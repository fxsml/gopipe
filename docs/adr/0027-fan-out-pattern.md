# ADR 0027: Fan-Out Pattern

**Date:** 2025-12-13
**Status:** Proposed
**Related:** ADR 0026 (Pipe Simplification), ADR 0022 (Internal Routing)

## Context

The current `channel.Broadcast` function provides basic fan-out capability, but we need to define clear semantics for fan-out in the context of:

1. Message processing pipelines
2. Pub/sub messaging systems
3. The new simplified pipe architecture (ADR 0026)

### Current Implementation

```go
// channel.Broadcast - sends each value to ALL output channels
func Broadcast[T any](in <-chan T, n int) []<-chan T
```

**Characteristics:**
- Creates n output channels at construction time
- Sends same value to all outputs (no copying for pointers)
- Blocks if ANY receiver is slow (back-pressure)
- No buffering, no timeout

### Fan-Out Use Cases in Messaging

| Use Case | Description | Pattern |
|----------|-------------|---------|
| **Event Broadcasting** | Notify multiple services of an event | Broadcast (all receivers) |
| **Audit Trail** | Log events alongside main processing | Broadcast (main + audit) |
| **CQRS Projections** | Update multiple read models from one event | Broadcast (all projections) |
| **Parallel Processing** | Process same message in parallel paths | Broadcast (concurrent) |
| **Load Balancing** | Distribute work across workers | Round-robin (one receiver) |
| **Topic Routing** | Route to subscribers by topic | Pub/Sub (filtered receivers) |

### Critical Analysis

**Is Fan-Out the Right Pattern?**

1. **For Event Broadcasting**: Yes - natural fit for pub/sub
2. **For Parallel Processing**: Consider - may be better as concurrent handlers
3. **For Load Balancing**: No - use `WithConcurrency` in ProcessorConfig instead
4. **For Topic Routing**: Partially - InternalRouter (ADR 0022) is more explicit

**Blocking Behavior Concern:**

The current implementation blocks on the slowest receiver. This can cause:
- Pipeline stalls if one consumer is slow
- Memory buildup if buffering is added
- Cascading back-pressure

## Decision

### 1. Rename and Clarify Terminology

| Term | Definition | Use Case |
|------|------------|----------|
| **Broadcast** | Same message to ALL consumers | Event notifications |
| **FanOut** | Generic term for 1→N distribution | Umbrella term |
| **Router** | Selective distribution by criteria | Topic-based routing |
| **LoadBalance** | Round-robin to ONE consumer | Work distribution |

Keep `Broadcast` name since it accurately describes "all receivers" semantics.

### 2. FanOutPipe Implementation

Add a `FanOutPipe` that integrates with the new `ProcessorConfig`:

```go
// FanOutPipe broadcasts messages to multiple downstream pipes
type FanOutPipe[In any] struct {
    outputs []Pipe[In, any]  // Downstream pipes
    config  FanOutConfig
}

type FanOutConfig struct {
    // Buffering
    Buffer int  // Per-output buffer, default: 0 (blocking)

    // Error handling
    OnSlowConsumer func(consumerIndex int, msg any) // Called when consumer blocks
    SlowThreshold  time.Duration                    // Time before considered slow

    // Behavior on error
    FailFast bool  // Stop all outputs if one fails, default: false
}

func NewFanOutPipe[In any](
    config FanOutConfig,
    outputs ...Pipe[In, any],
) Pipe[In, struct{}]
```

**Why `Pipe[In, struct{}]`?**

Fan-out doesn't produce output - it distributes to downstream pipes. The `struct{}` output signals "no output from this stage."

### 3. Broadcast Enhancement

Enhance `channel.Broadcast` with configuration:

```go
type BroadcastConfig struct {
    Buffer         int           // Per-output buffer, default: 0
    OnSlowReceiver func(index int, val any, elapsed time.Duration)
    SlowThreshold  time.Duration // Duration before OnSlowReceiver called
}

func BroadcastWithConfig[T any](
    in <-chan T,
    n int,
    config BroadcastConfig,
) []<-chan T
```

**Keep simple `Broadcast` function** for backward compatibility.

### 4. Message-Aware Fan-Out

For `*Message` types, provide message-aware fan-out:

```go
// MessageFanOut broadcasts CloudEvents to multiple destinations
func MessageFanOut(
    in <-chan *Message,
    destinations []string,  // e.g., ["gopipe://orders", "gopipe://audit"]
    config BroadcastConfig,
) map[string]<-chan *Message
```

**Relationship to Destination Attribute (ADR 0024):**

Fan-out at message level can set destination attribute:

```go
// Each output message gets destination set
for _, dest := range destinations {
    msg := event.Clone()
    msg.SetDestination(dest)
    outputs[dest] <- msg
}
```

### 5. When NOT to Use Fan-Out

| Scenario | Instead Use |
|----------|-------------|
| Concurrent processing of same logic | `ProcessorConfig{Concurrency: N}` |
| Route by message type | `InternalRouter` (ADR 0022) |
| Distribute work to workers | Single channel + concurrent processors |
| Conditional routing | Router with predicates |

### 6. Fan-Out vs Router Distinction

```
Fan-Out (Broadcast):          Router:
    ┌→ Consumer A                ┌→ Consumer A (if type == "order")
msg ├→ Consumer B            msg ─┼→ Consumer B (if type == "payment")
    └→ Consumer C                └→ Consumer C (if type == "shipping")
   (ALL get message)            (ONE gets message based on criteria)
```

**Clear rule:**
- **Fan-Out**: Message goes to ALL outputs
- **Router**: Message goes to ONE output (based on predicate)

### 7. Routing FanOut (Destination-Based)

For `message.Engine` (ADR 0029), we need a FanOut that routes to ONE output based on a routing function. This is distinct from Broadcast (all outputs) and complements Router (type-based).

```go
// RoutingFanOut routes each message to ONE output based on routing function
type RoutingFanOut[T any] struct {
    router   RoutingFunc[T]
    outputs  map[string]chan<- T
    config   RoutingFanOutConfig
}

// RoutingFunc determines the output key for a message
type RoutingFunc[T any] func(msg T) string

// RoutingFanOutConfig configures routing behavior
type RoutingFanOutConfig struct {
    DefaultOutput    string                              // fallback if no match
    OnNoMatch        func(msg any)                       // called when no route matches
    OnSlowConsumer   func(dest string, msg any, elapsed time.Duration)
    BufferPerOutput  int                                 // per-output buffer size
}

// Factory function
func NewRoutingFanOut[T any](
    router RoutingFunc[T],
    config RoutingFanOutConfig,
) *RoutingFanOut[T]

// AddOutput registers an output channel for a key/pattern
func (f *RoutingFanOut[T]) AddOutput(key string, out chan<- T)

// Start begins routing from input to outputs
func (f *RoutingFanOut[T]) Start(ctx context.Context, in <-chan T)
```

**Use Case: Destination-Based Routing for Engine**

```go
// Route messages by destination scheme
fanout := NewRoutingFanOut(
    func(msg *Message) string {
        dest := msg.Destination()
        if idx := strings.Index(dest, "://"); idx != -1 {
            return dest[:idx+3]  // "kafka://topic" → "kafka://"
        }
        return "default"
    },
    RoutingFanOutConfig{
        DefaultOutput: "gopipe://",
        BufferPerOutput: 100,
    },
)

// Register outputs for different destination schemes
fanout.AddOutput("kafka://", kafkaPublisherChan)
fanout.AddOutput("nats://", natsPublisherChan)
fanout.AddOutput("http://", httpPublisherChan)
fanout.AddOutput("gopipe://", internalLoopChan)

fanout.Start(ctx, routerOutput)
```

**Comparison Table:**

| Component | Routes To | Criteria | Use Case |
|-----------|-----------|----------|----------|
| **Broadcast** | ALL outputs | None | CQRS projections, audit |
| **Router** | ONE handler | Event type/topic | Handler dispatch |
| **RoutingFanOut** | ONE output | Custom function | Destination routing |

**When to Use RoutingFanOut:**

- Route by message destination attribute
- Route by custom criteria (not type-based)
- Engine's publisher/loop routing
- Multi-protocol publishing

## Usage Scenarios

### Scenario 1: Event Audit Trail

```go
// Every order event goes to processing AND audit
orders := subscription.Messages()
outputs := channel.Broadcast(orders, 2)

// Main processing pipeline
processed := pipe.Start(ctx, outputs[0])

// Audit pipeline (different handler, maybe different destination)
audited := auditPipe.Start(ctx, outputs[1])
```

### Scenario 2: CQRS Multiple Projections

```go
// Order events update multiple read models
events := eventStore.Stream(ctx, "orders", 0)
fanout := channel.Broadcast(events, 3)

// Each projection gets all events
orderListProjection.Start(ctx, fanout[0])
orderStatsProjection.Start(ctx, fanout[1])
customerOrdersProjection.Start(ctx, fanout[2])
```

### Scenario 3: Multi-Destination Publishing

```go
// Publish to multiple external systems
pipe := NewFanOutPipe(
    FanOutConfig{Buffer: 100},
    NewSendPipe("kafka://orders", kafkaSender),
    NewSendPipe("nats://orders.created", natsSender),
    NewSendPipe("http://webhook.example.com", httpSender),
)
pipe.Start(ctx, events)
```

### Scenario 4: Conditional Fan-Out (Anti-Pattern)

```go
// DON'T do this - use Router instead
fanout := channel.Broadcast(events, 3)
go func() {
    for msg := range fanout[0] {
        if msg.Type() == "order.created" { /* process */ }
    }
}()
// This wastes resources - each consumer filters

// DO this instead
router := NewInternalRouter()
router.Route("order.created", orderHandler)
router.Route("order.shipped", shipHandler)
```

## Rationale

### Why Keep Simple Broadcast?

1. **Simplicity**: Most use cases need basic 1→N without configuration
2. **Backward Compatibility**: Existing code continues to work
3. **Explicitness**: `BroadcastWithConfig` signals "I need special behavior"

### Why Not Add Load Balancing to Fan-Out?

Load balancing is fundamentally different:
- Fan-Out: ALL consumers get each message
- Load Balancing: ONE consumer gets each message

Mixing these concepts causes confusion. Instead:
- Use `ProcessorConfig{Concurrency: N}` for concurrent processing
- Use `channel.Merge` + multiple processors for work distribution

### Why Message-Aware Fan-Out?

CloudEvents have destination attribute. Message-aware fan-out can:
- Set destination per output
- Enable downstream routing decisions
- Integrate with pub/sub systems

## Consequences

### Positive

- Clear terminology (Broadcast vs Router vs LoadBalance)
- Enhanced Broadcast with slow-receiver handling
- FanOutPipe integrates with new ProcessorConfig
- Message-aware fan-out for CloudEvents

### Negative

- Two functions (`Broadcast` and `BroadcastWithConfig`) for same concept
- FanOutPipe returns `struct{}` which is unusual
- May need documentation to prevent misuse

### Migration

```go
// Old - still works
outputs := channel.Broadcast(in, 3)

// New - with configuration
outputs := channel.BroadcastWithConfig(in, 3, BroadcastConfig{
    Buffer:        10,
    SlowThreshold: time.Second,
    OnSlowReceiver: func(idx int, val any, elapsed time.Duration) {
        log.Printf("consumer %d slow: %v", idx, elapsed)
    },
})
```

## Links

- [ADR 0026: Pipe and Processor Simplification](0026-pipe-processor-simplification.md)
- [ADR 0022: Internal Message Routing](0022-internal-message-routing.md)
- [ADR 0029: Message Engine](0029-message-engine.md)
- [Feature 16: Core Pipe Refactoring](../features/16-core-pipe-refactoring.md)
- [Feature 17: Message Engine](../features/17-message-engine.md)
