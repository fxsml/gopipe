# ADR 0007: Saga Coordinator Pattern

**Date:** 2024-12-08
**Status:** Implemented

## Context

Multi-step workflows need a way to map events to commands. Event handlers returning commands directly couples them to specific command types. A separate coordinator decouples workflow logic from side effects.

## Decision

Implement SagaCoordinator interface for workflow orchestration:

```go
type SagaCoordinator interface {
    OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
```

Coordinators:
- Receive events
- Determine next commands in workflow
- Return commands to execute (or nil for terminal events)
- Keep workflow logic separate from event side effects

## Consequences

**Positive:**
- Decoupled: Event handlers don't know about commands
- Testable: Saga logic tested separately
- Flexible: Easy to modify workflow steps
- Supports one event triggering multiple commands

**Negative:**
- Additional abstraction to understand
- Coordinator must handle all relevant events

## Links

- ADR 0006: CQRS Implementation
- ADR 0008: Compensating Saga Pattern (proposed)
