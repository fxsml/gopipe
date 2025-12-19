# ADR 0006: CQRS Implementation

**Date:** 2025-12-08
**Status:** Implemented
**Updated:** 2025-12-11

> **Update 2025-12-11:** The `cqrs` package has been moved to `message/cqrs`. The core Router and
> Handler types are now in the `message` package, with CQRS-specific handlers in `message/cqrs`.

## Context

CQRS (Command Query Responsibility Segregation) separates read and write operations in event-driven architectures. gopipe had strong foundations (Router, Handler, JSON marshaling) but lacked explicit CQRS semantics and convenience APIs.

## Decision

Implement CQRS support in the `message/cqrs` package with:

```go
// Marshaler for command/event serialization
type Marshaler interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    Name(v any) string
}

// Command handler: Command -> Events
func NewCommandHandler[Cmd, Evt any](
    name string,
    marshaler Marshaler,
    handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) message.Handler

// Event handler: Event -> side effects
func NewEventHandler[Evt any](
    name string,
    marshaler Marshaler,
    handle func(ctx context.Context, evt Evt) error,
) message.Handler

// SagaCoordinator: Event -> Commands (workflow orchestration)
type SagaCoordinator interface {
    OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
```

Design principles: Type-safe, simple, flexible marshaling, explicit command/event distinction.

## Consequences

**Positive:**
- Type-safe CQRS with generics
- Pluggable serialization (JSON, Protobuf)
- Decoupled event handlers (side effects only)
- Saga coordination via interface

**Negative:**
- Additional package to learn
- Runtime type assertions in handlers

## Links

- ADR 0007: Saga Coordinator Pattern
- ADR 0008: Compensating Saga Pattern (proposed)
- ADR 0009: Transactional Outbox Pattern (proposed)
- Package: `github.com/fxsml/gopipe/message/cqrs`
