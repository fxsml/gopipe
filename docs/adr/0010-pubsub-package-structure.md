# ADR 0010: Pub/Sub Package Structure

**Date:** 2024-12-08
**Status:** Implemented

## Context

Broker implementation was in `message/broker/` with interfaces in `message/pubsub.go`. This caused naming confusion (`broker.NewBroker()`), mixed core message types with pub/sub concepts, and unclear API surface.

## Decision

Create dedicated `pubsub` package at top level:

```
pubsub/
├── broker.go       # Sender, Receiver, Broker interfaces
├── memory.go       # In-memory broker
├── channel.go      # Channel-based broker
├── io.go           # IO stream broker (JSONL)
├── http.go         # HTTP webhook broker
├── multiplex.go    # Routing between multiple brokers
├── publisher.go    # Publisher with batching
├── subscriber.go   # Subscriber with gopipe integration
└── topics.go       # Topic pattern matching
```

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
func (s *Subscriber) AddTopic(topic string)
func (s *Subscriber) Subscribe(ctx context.Context) <-chan *message.Message

type Publisher struct { ... }
func (p *Publisher) Publish(ctx, msgs) <-chan struct{}
```

### Subscriber Design

The Subscriber uses a two-phase approach:
1. **Topic Registration**: Call `AddTopic()` to register topics before subscribing
2. **Subscription**: Call `Subscribe()` to start polling all registered topics

Each topic is polled in a separate goroutine and messages are merged into a single output channel using `channel.Merge()`.

```go
subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
subscriber.AddTopic("orders.created")
subscriber.AddTopic("orders.updated")
msgs := subscriber.Subscribe(ctx)
```

This design:
- Allows subscribing to multiple topics with a single Subscribe call
- Each topic runs independently in its own goroutine
- All messages merge into one output channel for simplified consumption
- Supports configuration (concurrency, retry, timeout) per topic

## Consequences

**Positive:**
- Clear separation: core `message` vs `pubsub`
- Descriptive constructors: `NewInMemoryBroker()`, `NewChannelBroker()`
- Clean import paths
- Extensible: easy to add new broker implementations
- Subscriber supports multi-topic subscription with merged output

**Negative:**
- Breaking change: existing imports need updating
- More packages to understand

## Links

- Package: `github.com/fxsml/gopipe/pubsub`
- ADR 0012: Multiplex Pub/Sub
- Supersedes: ADR-25 Interface Broker
