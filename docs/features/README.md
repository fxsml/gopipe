# gopipe Pub/Sub Features

This directory contains feature documentation for the pub/sub branch, organized for systematic integration into the main branch.

## Feature Overview

These features represent the complete pub/sub implementation for gopipe, grouped logically by package and dependency order.

### Essential Features (Implemented)

1. **[Channel GroupBy](01-channel-groupby.md)** - Prerequisite for message batching
2. **[Message Core Refactor](02-message-core-refactor.md)** - Foundation for all message operations
3. **[Message Pub/Sub](03-message-pubsub.md)** - Publisher, Subscriber, and Broker implementations
4. **[Message Router](04-message-router.md)** - Message dispatch and handling
5. **[Message CQRS](05-message-cqrs.md)** - Command/Event handlers (core implemented)
6. **[Message CloudEvents](06-message-cloudevents.md)** - CloudEvents v1.0.2 protocol support
7. **[Message Multiplex](07-message-multiplex.md)** - Topic-based routing
8. **[Middleware Package](08-middleware-package.md)** - Reusable middleware components

### Proposed Features (Future Work)

These features are documented but not yet implemented:
- **Saga Coordinator** - Multi-step workflow orchestration (ADR 0007)
- **Compensating Saga** - Rollback for failed workflows (ADR 0008)
- **Transactional Outbox** - Reliable event publishing (ADR 0009)

## Integration Order

Features are numbered in dependency order. When integrating into main:

1. Start with **01-channel-groupby** (prerequisite)
2. Continue with **02-message-core-refactor** (foundation)
3. Add **03-message-pubsub** (uses 01 and 02)
4. Add **04-message-router** (uses 02)
5. Add **05-message-cqrs** (uses 02 and 04)
6. Add **06-message-cloudevents** (uses 02)
7. Add **07-message-multiplex** (uses 03)
8. Add **08-middleware-package** (uses 04)

## Feature Document Structure

Each feature document includes:
- **Summary** - What the feature provides
- **Implementation** - Key types and APIs
- **Usage Examples** - How to use the feature
- **Files Changed** - What was added/modified
- **Related Features** - Dependencies and relationships
- **Related ADRs** - Architecture decisions

## ADR Reference

Architecture Decision Records are organized by status:

### Implemented
- [ADR 0001](../adr/0001-public-message-fields.md) - Public Message Fields
- [ADR 0002](../adr/0002-remove-properties-thread-safety.md) - Remove Properties Thread Safety
- [ADR 0003](../adr/0003-remove-noisy-properties.md) - Remove Noisy Properties
- [ADR 0004](../adr/0004-dual-message-types.md) - Dual Message Types
- [ADR 0005](../adr/0005-remove-functional-options.md) - Remove Functional Options
- [ADR 0006](../adr/0006-cqrs-implementation.md) - CQRS Implementation
- [ADR 0010](../adr/0010-pubsub-package-structure.md) - Pub/Sub Package Structure
- [ADR 0012](../adr/0012-multiplex-pubsub.md) - Multiplex Pub/Sub
- [ADR 0013](../adr/0013-processor-abstraction.md) - Processor Abstraction
- [ADR 0014](../adr/0014-composable-pipe.md) - Composable Pipe
- [ADR 0015](../adr/0015-middleware-pattern.md) - Middleware Pattern
- [ADR 0016](../adr/0016-channel-package.md) - Channel Package
- [ADR 0017](../adr/0017-message-acknowledgment.md) - Message Acknowledgment

### Accepted
- [ADR 0018](../adr/0018-cloudevents-terminology.md) - CloudEvents Terminology

### Proposed
- [ADR 0007](../adr/0007-saga-coordinator-pattern.md) - Saga Coordinator Pattern
- [ADR 0008](../adr/0008-compensating-saga-pattern.md) - Compensating Saga Pattern
- [ADR 0009](../adr/0009-transactional-outbox-pattern.md) - Transactional Outbox Pattern

### Superseded
- [ADR 0011](../adr/0011-cloudevents-compatibility.md) - Superseded by ADR 0018

## Testing

All implemented features have comprehensive test coverage. Run all tests:

```bash
go test ./...
```

Run specific package tests:
```bash
go test ./channel -v
go test ./message -v
go test ./message/broker -v
go test ./message/cqrs -v
go test ./middleware -v
```

## Examples

Working examples demonstrating features:
- `examples/broker/` - Broker implementations (channel, HTTP, IO)
- `examples/cqrs-package/` - CQRS handlers and router
- `examples/cqrs-saga-coordinator/` - Saga pattern implementation
- `examples/multiplex-pubsub/` - Topic-based routing
- `examples/router-middleware/` - Middleware usage

## Migration from feat/pubsub to main

This documentation supports a systematic merge strategy:

1. **Feature-by-feature integration** - Each feature can be cherry-picked independently
2. **Squash per feature** - Related commits squashed into single feature commits
3. **Clear git history** - One commit per feature with reference to feature doc
4. **Testable increments** - Tests pass after each feature integration

See [CLAUDE.md](../../CLAUDE.md) for the complete integration procedure.
