# ADR 0012: Multiplex Publisher/Subscriber

**Date:** 2025-12-08
**Status:** Implemented
**Updated:** 2025-12-11

> **Update 2025-12-11:** Package moved from `pubsub/multiplex` to `message/multiplex`.

## Context

Applications often need to route messages to different broker implementations based on topic patterns. Use cases: internal vs external messages, audit logs to dedicated systems, environment-specific routing, cost optimization.

## Decision

Implement multiplexing layer in `message/multiplex` package for routing to different Senders/Receivers:

```go
// message/multiplex package
type SenderSelector func(topic string) message.Sender
type ReceiverSelector func(topic string) message.Receiver

type Sender struct {
    selector SenderSelector
    fallback message.Sender  // Required, never nil
}

type Receiver struct {
    selector ReceiverSelector
    fallback message.Receiver  // Required, never nil
}

// Helper selectors
func PrefixSenderSelector(prefix string, sender message.Sender) SenderSelector
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

- ADR 0010: Message Package Structure
- Package: `github.com/fxsml/gopipe/message/multiplex`
