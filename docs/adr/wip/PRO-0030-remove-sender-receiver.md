# ADR 0030: Remove Sender and Receiver Interfaces

**Date:** 2025-12-15
**Status:** Proposed
**Supersedes:** ADR 0010 (Pub/Sub Package Structure)

## Context

gopipe currently defines `Sender` and `Receiver` interfaces for pub/sub operations:

```go
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*Message) error
}

type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*Message, error)
}
```

These interfaces are used by:
- `Publisher` (wraps Sender with batching via `channel.GroupBy`)
- `Subscriber` (wraps Receiver with `gopipe.Generator`)
- `broker/http.go` (HTTPSender, HTTPReceiver)
- `broker/io.go` (IOSender, IOReceiver)
- `multiplex/multiplex.go` (routing wrappers)

### Problems Identified

1. **Forces poll-based model**: `Receive()` returns a slice, requiring polling. Modern brokers (Kafka, NATS, RabbitMQ) are push-based with callbacks or channels.

2. **Topic parameter is too simplistic**:
   - Kafka: partition-aware, async with callbacks
   - NATS: subjects with wildcards, request-reply, JetStream
   - RabbitMQ: exchange/queue model with confirms
   - The single `topic string` parameter doesn't map cleanly

3. **Redundant with Subscriber interface**: ADR 0028 introduces `Subscriber[Out any]` which returns `<-chan Out` - more Go-idiomatic and fits push-based brokers.

4. **Publishing is trivial without abstraction**:
   ```go
   // With channel.GroupBy, publishing is straightforward
   groups := channel.GroupBy(msgs, topicFunc, config)
   for group := range groups {
       kafkaProducer.Send(group.Key, group.Items)
   }
   ```

5. **Constrains adapter implementations**: HTTPReceiver must buffer messages internally, breaking backpressure. IOReceiver uses timeouts to simulate polling.

6. **Unnecessary abstraction layer**: Publisher and Subscriber add complexity wrapping these interfaces when the underlying operations are simple.

## Decision

**Remove Sender and Receiver interfaces.** Deprecate Publisher and Subscriber wrappers.

### Rationale

1. **gopipe should provide primitives, not broker contracts**:
   - `channel.GroupBy` for batching by topic
   - `channel.Merge` for combining sources
   - `Subscriber[Out]` (ADR 0028) as standard input interface
   - Let broker adapters implement what makes sense

2. **Broker adapters gain freedom**:
   - Kafka adapter can use partition-aware async API
   - NATS adapter can use push-based subscriptions
   - No forced polling or topic string constraints

3. **Simpler mental model**:
   ```
   Before: Broker → Receiver → Subscriber → <-chan Message
   After:  Broker → Subscriber[*Message] → <-chan Message (direct)

   Before: <-chan Message → Publisher → Sender → Broker
   After:  <-chan Message → GroupBy → Broker (direct)
   ```

### Migration Path

**Consuming messages:**
```go
// Before
receiver := kafka.NewReceiver(config)
subscriber := message.NewSubscriber(receiver, subConfig)
msgs := subscriber.Subscribe(ctx, "orders")

// After: Broker implements Subscriber[*Message] directly
subscriber := kafka.NewSubscriber(config)
msgs := subscriber.Subscribe(ctx)  // topic in config
```

**Publishing messages:**
```go
// Before
sender := kafka.NewSender(config)
publisher := message.NewPublisher(sender, pubConfig)
publisher.Publish(ctx, msgs)

// After: Use channel.GroupBy + broker client
groups := channel.GroupBy(msgs, func(m *Message) string {
    topic, _ := m.Attributes.Topic()
    return topic
}, channel.GroupByConfig{MaxBatchSize: 100})

for group := range groups {
    kafkaProducer.Send(ctx, group.Key, group.Items)
}
```

### What Stays

- `channel.GroupBy` - essential for batching by topic
- `Subscriber[Out any]` (ADR 0028) - standard source interface
- `Router` - message dispatch
- Broker-specific implementations in external packages

### What Gets Deprecated

| Component | Status | Migration |
|-----------|--------|-----------|
| `Sender` interface | **Deprecated** | Use broker client directly with GroupBy |
| `Receiver` interface | **Deprecated** | Implement `Subscriber[*Message]` |
| `Publisher` | **Deprecated** | Use `channel.GroupBy` + broker |
| `Subscriber` (wrapper) | **Deprecated** | Use `Subscriber[*Message]` interface |
| `broker/http.go` | **Move to examples** | Reference implementation |
| `broker/io.go` | **Move to examples** | Debug/testing tool |
| `multiplex/` | **Deprecated** | Use Engine routing (ADR 0029) |

## Consequences

### Positive

1. **Reduced complexity**: Fewer interfaces, less indirection
2. **Broker freedom**: Adapters implement what fits their model
3. **Better fit for push-based brokers**: No forced polling
4. **Clearer responsibility**: gopipe = primitives, adapters = integration
5. **Simpler publishing**: `channel.GroupBy` is sufficient

### Negative

1. **Breaking change**: Existing code using Sender/Receiver must migrate
2. **Less standardization**: No common broker interface
3. **More code in adapters**: Each broker handles its own patterns

### Mitigations

1. **Deprecation period**: Mark as deprecated before removal
2. **Migration guide**: Document clear migration paths
3. **Example adapters**: Provide reference implementations
4. **Subscriber[*Message]**: Standard interface for consuming

## Implementation

### Phase 1: Deprecate
```go
// Deprecated: Use broker-specific Subscriber[*Message] implementation.
// See ADR 0030 for migration guide.
type Sender interface { ... }

// Deprecated: Implement Subscriber[*Message] directly.
// See ADR 0030 for migration guide.
type Receiver interface { ... }
```

### Phase 2: Move broker/ to examples/
- `broker/http.go` → `examples/http-broker/`
- `broker/io.go` → `examples/io-broker/`

### Phase 3: Remove (next major version)
- Remove Sender, Receiver interfaces
- Remove Publisher, Subscriber wrappers
- Remove multiplex package

## Links

- [ADR 0010: Pub/Sub Package Structure](0010-pubsub-package-structure.md) - Superseded
- [ADR 0028: Subscriber Patterns](0028-generator-source-patterns.md) - Subscriber interface
- [ADR 0029: Message Engine](0029-message-engine.md) - Replaces multiplex routing
- [channel.GroupBy](../features/01-channel-groupby.md) - Batching primitive
