# ADR 0012: Message Package Structure

**Date:** 2025-12-08
**Status:** Implemented

## Context

Initially, broker implementation was in `message/broker/` with interfaces in `message/pubsub.go`. This caused naming confusion (`broker.NewBroker()`), mixed core message types with pub/sub concepts, and unclear API surface.

A separate `pubsub` package was created, but this added unnecessary separation between closely related concepts. The `Sender` and `Receiver` interfaces are core to message handling and belong with the `Message` type.

## Decision

Keep the `Sender` and `Receiver` interfaces in the `message` package alongside the core `Message` type. Publisher and Subscriber are also part of the `message` package. Broker implementations, multiplex routing, and CloudEvents support are in subpackages:

```
message/
├── message.go      # Message, Attributes, Sender, Receiver interfaces
├── handler.go      # Handler interface, Matcher, basic matchers
├── router.go       # Router for message dispatch
├── publisher.go    # Publisher with batching
├── subscriber.go   # Subscriber with gopipe integration
├── broker/         # Broker implementations
│   ├── channel.go  # Channel-based in-process broker
│   ├── http.go     # HTTP webhook broker (CloudEvents)
│   └── io.go       # IO broker for debugging/bridging (JSONL)
├── multiplex/      # Topic-based routing
│   └── multiplex.go # Sender/Receiver routing by topic
├── cloudevents/    # CloudEvents serialization
│   └── cloudevents.go
└── cqrs/           # CQRS command/event handlers
    ├── handler.go  # NewCommandHandler, NewEventHandler
    ├── marshaler.go # CommandMarshaler, EventMarshaler
    └── attributes.go # AttributeProvider utilities
```

### IO Broker (Debug/Management)

The IO broker is designed for debugging, logging, and bridging - not as a production
message broker. It serializes messages as JSONL (one CloudEvent per line).

Use cases:
- Debug logging: Subscribe from a real broker, write to file for inspection
- Replay testing: Read from file, publish to a real broker
- Pipe-based IPC: stdin/stdout communication between processes

Topic handling:
- **Send**: Writes all messages; topic preserved in CloudEvent "topic" extension
- **Receive**: Empty topic returns all; non-empty topic filters by exact match

Core interfaces and types (in `message` package):
```go
// Pub/sub interfaces
type Sender interface {
    Send(ctx context.Context, topic string, msgs []*Message) error
}

type Receiver interface {
    Receive(ctx context.Context, topic string) ([]*Message, error)
}

type Broker interface { Sender; Receiver }

// Handler and routing
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
    Match(attrs Attributes) bool
}

type Matcher func(Attributes) bool

type Router struct { ... }
func NewRouter(config RouterConfig, handlers ...Handler) *Router
func (r *Router) Start(ctx context.Context, msgs <-chan *Message) <-chan *Message

// Publisher/Subscriber
type Subscriber struct { ... }
func (s *Subscriber) Subscribe(ctx context.Context, topic string) <-chan *Message

type Publisher struct { ... }
func (p *Publisher) Publish(ctx, msgs) <-chan struct{}
```

### Subscriber Design

The Subscriber uses a simple per-topic approach:
1. **Subscription**: Call `Subscribe(ctx, topic)` for each topic you want to consume
2. **Merging**: Use `channel.Merge()` to combine multiple topic channels if needed

```go
subscriber := message.NewSubscriber(broker, message.SubscriberConfig{})
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

**Breaking Changes:**
- Existing imports need updating (from `pubsub` to `message`)

**Benefits:**
- Unified package: core `Message`, `Sender`, `Receiver` all in `message`
- Clear hierarchy: `message/broker`, `message/multiplex`, `message/cloudevents`
- Descriptive constructors: `broker.NewChannelBroker()`, `broker.NewHTTPSender()`
- Extensible: easy to add new broker implementations
- Subscriber supports multi-topic subscription with merged output

**Drawbacks:**
- More subpackages to understand

## Links

- Related: ADR 0011 (CQRS Implementation)
- Related: ADR 0013 (Multiplex Publisher/Subscriber)
- Package: `github.com/fxsml/gopipe/message`

## Updates

**2025-12-10:** Subscriber API simplified from `AddTopic()` + `Subscribe()` to `Subscribe(ctx, topic)` per topic.

**2025-12-11:** Merged pub/sub into `message` package. Removed separate `pubsub` package. Router, Handler, Matcher moved to `message`.

**2025-12-22:** Updated Consequences format to match ADR template.
