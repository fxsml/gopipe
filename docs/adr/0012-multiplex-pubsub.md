# ADR 0012: Multiplex Publisher/Subscriber

## Status
Accepted

## Context
Applications often need to route messages to different broker implementations based on message characteristics (topic patterns, metadata, etc.). Common use cases include:

1. **Performance Optimization**: Route high-throughput internal messages to an in-memory broker for speed, while sending external messages to a durable broker like NATS or Kafka.

2. **Separation of Concerns**: Route audit logs to a dedicated audit system, operational metrics to a monitoring system, and business events to the main message bus.

3. **Environment-Specific Routing**: In development, route all messages to an in-memory broker for simplicity; in production, use sophisticated routing rules.

4. **Cost Optimization**: Keep low-priority or internal messages in cheaper/faster brokers, send critical messages to more expensive/reliable brokers.

Without multiplexing, applications would need to:
- Manually implement routing logic in application code
- Manage multiple Publisher/Subscriber instances
- Duplicate routing decisions across the codebase

## Decision
We will implement a multiplexing layer for pubsub that allows routing messages to different Sender/Receiver implementations based on configurable logic.

### Core Types

```go
// SenderSelector determines which Sender to use for a given topic
type SenderSelector func(topic string) Sender

// ReceiverSelector determines which Receiver to use for a given topic
type ReceiverSelector func(topic string) Receiver

// MultiplexSender routes Send calls to different Sender implementations
type MultiplexSender struct {
    selector SenderSelector
    fallback Sender  // Required, never nil
}

// MultiplexReceiver routes Receive calls to different Receiver implementations
type MultiplexReceiver struct {
    selector ReceiverSelector
    fallback Receiver  // Required, never nil
}
```

### Design Principles

1. **Function-Based Selectors**:
   - Simple, flexible, follows Go idioms (like `http.HandlerFunc`)
   - Easy to compose with helpers
   - Allows custom logic without interfaces

2. **Required Fallback**:
   - Ensures deterministic routing (no unhandled topics)
   - Forces users to think about default behavior
   - Prevents runtime errors from missing routes

3. **Selector Returns nil for No Match**:
   - Clear semantics: `nil` = use fallback
   - Simpler than returning errors
   - Error handling happens at `Send()`/`Receive()` level

4. **Pattern Matching Syntax**:
   - `*` = single segment wildcard (`orders.*` matches `orders.created`)
   - `**` = multi-segment wildcard (`audit.**` matches `audit.us.log`)
   - Familiar to users of NATS, RabbitMQ, glob patterns

5. **First Match Wins**:
   - Routes evaluated in order
   - Predictable behavior
   - Allows specific → general patterns

### Helper Functions

Provide common routing patterns out of the box:

```go
// Pattern-based routing
NewTopicSenderSelector(routes []TopicSenderRoute) SenderSelector
NewTopicReceiverSelector(routes []TopicReceiverRoute) ReceiverSelector

// Simple prefix matching
PrefixSenderSelector(prefix string, sender Sender) SenderSelector
PrefixReceiverSelector(prefix string, receiver Receiver) ReceiverSelector

// Chain multiple selectors (first match wins)
ChainSenderSelectors(selectors ...SenderSelector) SenderSelector
ChainReceiverSelectors(selectors ...ReceiverSelector) ReceiverSelector
```

### Usage Example

```go
// Create brokers
memoryBroker := pubsub.NewInMemoryBroker(pubsub.InMemoryConfig{})
natsBroker := createNATSBroker()
auditBroker := createAuditBroker()

// Define routing rules
selector := pubsub.NewTopicSenderSelector([]pubsub.TopicSenderRoute{
    {Pattern: "internal.*", Sender: memoryBroker},
    {Pattern: "audit.**", Sender: auditBroker},
})

// Create multiplex sender with fallback
multiplexSender := pubsub.NewMultiplexSender(selector, natsBroker)

// Use with Publisher
publisher := pubsub.NewPublisher(
    multiplexSender,
    pubsub.RouteBySubject(),
    pubsub.PublisherConfig{},
)

// Messages automatically routed based on topic:
// - "internal.cache" → memoryBroker
// - "audit.security.login" → auditBroker
// - "orders.created" → natsBroker (fallback)
```

## Consequences

### Positive

1. **Flexible Routing**: Applications can route to different brokers based on any criteria (topic, properties, content, environment, etc.).

2. **Performance Optimization**: Easy to route high-throughput messages to fast in-memory brokers while sending critical messages to durable brokers.

3. **Separation of Concerns**: Clean separation between routing logic and business logic.

4. **Composable**: Works seamlessly with existing Publisher/Subscriber/Broker implementations.

5. **Zero Breaking Changes**: Pure additive feature, existing code continues to work.

6. **Testable**: Easy to unit test routing logic separately from broker implementations.

7. **Zero Overhead**: Minimal abstraction cost (single function call + delegation).

### Negative

1. **Complexity**: Adds another layer of abstraction that users need to understand.

2. **Debug Difficulty**: May be harder to trace which broker handled a message when debugging.

3. **Pattern Matching Limitations**: Current wildcards (`*`, `**`) cover most cases but not all (no regex support yet).

### Mitigations

1. **Complexity**: Comprehensive documentation and examples showing common use cases.

2. **Debug Difficulty**: Could add optional logging middleware that logs routing decisions.

3. **Pattern Limitations**: Keep simple patterns for 90% use case, users can write custom selectors for complex cases.

## Alternatives Considered

### 1. Interface-Based Selectors

```go
type SelectorInterface interface {
    Select(topic string) Sender
}
```

**Rejected**: Functions are simpler and more flexible. Interfaces add boilerplate without benefit for this use case.

### 2. Configuration-Based Routing

```go
type RouteConfig struct {
    Rules []RouteRule
}

type RouteRule struct {
    Pattern string
    Broker  string  // Name of broker
}
```

**Rejected**: Too rigid. Requires pre-registering brokers by name, doesn't support dynamic logic, harder to test.

### 3. Optional Fallback

```go
func NewMultiplexSender(selector SenderSelector, fallback Sender) *MultiplexSender {
    // fallback can be nil
}
```

**Rejected**: Allowing nil fallback could cause runtime panics when no route matches. Required fallback forces users to think about default behavior and makes routing deterministic.

### 4. Selector Returns Error

```go
type SenderSelector func(topic string) (Sender, error)
```

**Rejected**: Adds complexity without benefit. Routing decision is not an error-prone operation. Actual errors (connection failures, etc.) happen at `Send()` level.

## Related Decisions

- **ADR-0010**: Pubsub Package Structure - Multiplexing builds on the existing Sender/Receiver abstractions.
- **ADR-0006**: CQRS Implementation - Pattern matching syntax inspired by event routing needs.

## Future Enhancements

Potential future additions (not in initial scope):

1. **Regex Pattern Support**: For complex topic matching beyond wildcards
2. **Metric Collection**: Track which broker handles how many messages
3. **Health Checks**: Monitor broker availability, automatic failover
4. **Weighted Routing**: Distribute load across multiple brokers
5. **Topic Rewriting**: Transform topic names during routing
6. **Broadcast Mode**: Send message to multiple brokers simultaneously
7. **Conditional Routing**: Route based on message properties, not just topic

## References

- Implementation: `pubsub/multiplex.go`
- Tests: `pubsub/multiplex_test.go`
- Example: `examples/multiplex-pubsub/main.go`
- Plan: `.claude/plans/multiplex-pubsub.md`
