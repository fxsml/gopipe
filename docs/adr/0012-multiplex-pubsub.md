# ADR 0012: Multiplex Publisher/Subscriber

**Date:** 2024-12-08
**Status:** Implemented

## Context

Applications often need to route messages to different broker implementations based on topic patterns. Use cases: internal vs external messages, audit logs to dedicated systems, environment-specific routing, cost optimization.

## Decision

Implement multiplexing layer for routing to different Senders/Receivers:

```go
type SenderSelector func(topic string) Sender
type ReceiverSelector func(topic string) Receiver

type MultiplexSender struct {
    selector SenderSelector
    fallback Sender  // Required, never nil
}

type MultiplexReceiver struct {
    selector ReceiverSelector
    fallback Receiver  // Required, never nil
}

// Helper selectors
func PrefixSenderSelector(prefix string, sender Sender) SenderSelector
func NewTopicSenderSelector(routes []TopicSenderRoute) SenderSelector
func ChainSenderSelectors(selectors ...SenderSelector) SenderSelector
```

Pattern matching: "*" matches one segment, "**" matches multiple (dot-separated). First match wins.

## Consequences

**Positive:**
- Centralized routing logic
- Function-based selectors (flexible, composable)
- Required fallback prevents routing errors
- Easy to test (inject mocks via selector)

**Negative:**
- Additional abstraction layer
- Pattern matching has performance cost
- Selector must be stateless

## Links

- ADR 0010: Pub/Sub Package Structure
- Package: `github.com/fxsml/gopipe/pubsub`
