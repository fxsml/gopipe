# ADR 0011: CQRS Implementation

**Date:** 2025-12-08
**Status:** Implemented

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

**Benefits:**
- Type-safe CQRS with generics
- Pluggable serialization (JSON, Protobuf)
- Decoupled event handlers (side effects only)
- Saga coordination via interface

**Drawbacks:**
- Additional package to learn
- Runtime type assertions in handlers

## Links

- Related: ADR 0012 (Message Package Structure)
- Package: `github.com/fxsml/gopipe/message/cqrs`

## Updates

**2025-12-11:** Package moved from `cqrs` to `message/cqrs`.

**2025-12-22:** Fixed Links references. Updated Consequences format to match ADR template.
