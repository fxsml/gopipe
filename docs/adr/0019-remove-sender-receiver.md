# ADR 0019: Remove Sender and Receiver Interfaces

**Date:** 2025-12-22
**Status:** Proposed (see [ADR 0022](0022-message-package-redesign.md))

## Context

Current `Sender` and `Receiver` interfaces force a poll-based model:

```go
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*Message) error
}

type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*Message, error)
}
```

**Problems:**
1. Modern brokers (Kafka, NATS) are push-based, not poll-based
2. Single `topic string` doesn't map to partition-aware or exchange/queue models
3. Redundant with `Subscriber` interface that returns `<-chan *Message`
4. Publishing is trivial with `channel.GroupBy` + broker client

## Decision

**Remove Sender/Receiver.** Use simpler patterns:

### Consuming (Push-Based)

```go
// Before: Receiver → Subscriber wrapper
receiver := kafka.NewReceiver(config)
subscriber := message.NewSubscriber(receiver, subConfig)
msgs := subscriber.Subscribe(ctx, "orders")

// After: Broker implements channel-returning interface directly
subscriber := kafka.NewSubscriber(config)
msgs := subscriber.Subscribe(ctx)  // topic in config
```

### Publishing (GroupBy + Broker)

```go
// Before: Publisher → Sender wrapper
sender := kafka.NewSender(config)
publisher := message.NewPublisher(sender, pubConfig)
publisher.Publish(ctx, msgs)

// After: channel.GroupBy + broker client
groups := channel.GroupBy(msgs, topicFunc, config)
for group := range groups {
    broker.Send(ctx, group.Key, group.Items)
}
```

### Migration Guide

| Old Pattern | New Pattern |
|-------------|-------------|
| `Sender` interface | Broker client directly or `channel.GroupBy` |
| `Receiver` interface | Broker implements `Subscribe() <-chan *Message` |
| `Publisher` wrapper | `ce.NewPublisher(client).Publish(ctx, ch)` |
| `Subscriber` wrapper | `ce.NewSubscriber(client).Subscribe(ctx, topic)` |
| `broker/` package | Use `message/cloudevents/` adapters |
| `multiplex/` routing | Engine output Matcher routing |

## Consequences

**Benefits:**
- Broker adapters implement what fits their model
- No forced polling for push-based brokers
- Simpler: `channel.GroupBy` sufficient for publishing
- Cleaner: `<-chan *Message` is the standard input

**Drawbacks:**
- Breaking change for existing code
- Less standardization across brokers

## Links

- Related: ADR 0018 (Interface Naming Conventions)
- Related: ADR 0020 (Message Engine Architecture)
- Related: ADR 0021 (Marshaler Pattern)
