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
```

## Consequences

**Positive:**
- Clear separation: core `message` vs `pubsub`
- Descriptive constructors: `NewInMemoryBroker()`, `NewChannelBroker()`
- Clean import paths
- Extensible: easy to add new broker implementations

**Negative:**
- Breaking change: existing imports need updating
- More packages to understand

## Links

- Package: `github.com/fxsml/gopipe/pubsub`
- ADR 0012: Multiplex Pub/Sub
- Supersedes: ADR-25 Interface Broker
