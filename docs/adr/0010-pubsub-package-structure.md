# ADR 0010: Pub/Sub Package Structure

**Date:** 2025-12-08
**Status:** Implemented

> **Historical Note:** The Subscriber API was simplified from the proposed multi-topic design.
> Instead of `AddTopic()` + `Subscribe()`, the actual API uses `Subscribe(ctx, topic)` per topic.
> Multiple topics can be subscribed by calling `Subscribe` multiple times and merging channels.

## Context

Broker implementation was in `message/broker/` with interfaces in `message/pubsub.go`. This caused naming confusion (`broker.NewBroker()`), mixed core message types with pub/sub concepts, and unclear API surface.

## Decision

Create dedicated `pubsub` package at top level with broker implementations in a subpackage:

```
pubsub/
├── broker.go       # Sender, Receiver, Broker interfaces
├── publisher.go    # Publisher with batching
├── subscriber.go   # Subscriber with gopipe integration
├── broker/         # Broker implementations
│   ├── channel.go  # Channel-based in-process broker
│   ├── http.go     # HTTP webhook broker (CloudEvents)
│   └── io.go       # IO broker for debugging/bridging (JSONL)
├── multiplex/      # Topic-based routing
│   └── multiplex.go # Sender/Receiver routing by topic
└── cloudevents/    # CloudEvents serialization
    └── cloudevents.go
```

### IO Broker (Debug/Management)

The IO broker is designed for debugging, logging, and bridging - not as a production
message broker. It serializes messages as JSONL (one CloudEvent per line).

Use cases:
- Debug logging: Subscribe from a real broker, write to file for inspection
- Replay testing: Read from file, publish to a real broker
- Pipe-based IPC: stdin/stdout communication between processes

Topic handling:
- **Send**: Writes all messages; topic preserved in CloudEvent extension
- **Receive**: Empty topic returns all; non-empty topic filters by exact match

Core interfaces:
```go
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*message.Message) error
}

type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

type Broker interface { Sender; Receiver }

type Subscriber struct { ... }
func (s *Subscriber) Subscribe(ctx context.Context, topic string) <-chan *message.Message

type Publisher struct { ... }
func (p *Publisher) Publish(ctx, msgs) <-chan struct{}
```

### Subscriber Design

The Subscriber uses a simple per-topic approach:
1. **Subscription**: Call `Subscribe(ctx, topic)` for each topic you want to consume
2. **Merging**: Use `channel.Merge()` to combine multiple topic channels if needed

```go
subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
orders := subscriber.Subscribe(ctx, "orders.created")
payments := subscriber.Subscribe(ctx, "payments.completed")
msgs := channel.Merge(orders, payments)
```

This design:
- Simple API: one method for subscription
- Independent subscriptions with separate goroutines per topic
- Flexible merging via `channel.Merge()` when needed
- Supports configuration (concurrency, retry, timeout) at subscriber level

## Consequences

**Positive:**
- Clear separation: core `message` vs `pubsub` vs `pubsub/broker`
- Descriptive constructors: `broker.NewChannelBroker()`, `broker.NewHTTPSender()`
- Clean import paths: interfaces in `pubsub`, implementations in `pubsub/broker`
- Extensible: easy to add new broker implementations
- Subscriber supports multi-topic subscription with merged output

**Negative:**
- Breaking change: existing imports need updating
- More packages to understand

## Links

- Interfaces: `github.com/fxsml/gopipe/pubsub`
- Broker implementations: `github.com/fxsml/gopipe/pubsub/broker`
- Multiplex routing: `github.com/fxsml/gopipe/pubsub/multiplex`
- ADR 0012: Multiplex Pub/Sub
- Supersedes: ADR-25 Interface Broker
