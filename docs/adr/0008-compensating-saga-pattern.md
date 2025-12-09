# ADR 0008: Compensating Saga Pattern

**Date:** 2024-12-08
**Status:** Proposed

## Context

When a saga step fails mid-workflow, previously completed steps may need to be undone. Basic saga coordinators don't track compensations. A compensating saga stores undo actions for each step and executes them in reverse order (LIFO) on failure.

## Decision

Extend saga coordination with compensation support:

```go
type CompensatingSagaCoordinator interface {
    SagaCoordinator
    OnEventWithCompensation(ctx context.Context, msg *message.Message) (SagaStep, error)
}

type SagaStep struct {
    Commands      []*message.Message  // Forward commands
    Compensations []*message.Message  // Undo commands (stored)
    IsTerminal    bool
}

type SagaStore interface {
    Save(ctx context.Context, state *SagaState) error
    Load(ctx context.Context, sagaID string) (*SagaState, error)
    Delete(ctx context.Context, sagaID string) error
}
```

On failure: enter COMPENSATING state, execute stored compensations in reverse.

## Consequences

**Positive:**
- Automatic rollback on failure
- Saga state visibility
- Graceful failure handling

**Negative:**
- Requires saga state store (in-memory or persistent)
- More complex than basic saga coordinator
- Compensation logic must be defined upfront

## Links

- ADR 0006: CQRS Implementation
- ADR 0007: Saga Coordinator Pattern
- Future package: `cqrs/compensation`
