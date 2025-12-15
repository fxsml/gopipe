# gopipe Pub/Sub Features

This directory contains feature documentation for the pub/sub branch, organized for systematic integration into the main branch.

**For the big picture:** See the [Architecture Roadmap](../plans/architecture-roadmap.md) for how these features fit together.

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

### Core Pipe Refactoring (Proposed - Prerequisite)

This is a **prerequisite** for CloudEvents standardization. Must be implemented first.

16. **[Core Pipe Refactoring](16-core-pipe-refactoring.md)** - ProcessorConfig, simplified cancel, Subscriber interface (ADRs 0026-0028)
17. **[Message Engine](17-message-engine.md)** - Top-level orchestration with FanIn/FanOut (ADR 0029)

### CloudEvents Standardization (Proposed)

Proposed features for making CloudEvents mandatory and enabling composable internal pipelines.
See [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md) for complete details.

9. **[CloudEvents Mandatory](09-cloudevents-mandatory.md)** - Enforce required CE attributes (ADR 0019)
10. **[Non-Generic Message](10-non-generic-message.md)** - Data as `any`, remove generics (ADR 0020)
11. **[ContentType Serialization](11-contenttype-serialization.md)** - Auto serialization at boundaries (ADR 0021)
12. **[Internal Message Routing](12-internal-message-routing.md)** - Topic-based internal routing (ADR 0022)
13. **[Internal Message Loop](13-internal-message-loop.md)** - Feedback loop with pluggable transport (ADR 0023)
14. **[NATS Integration](14-nats-integration.md)** - External package for advanced messaging (External)
15. **[SQL Event Store](15-sql-event-store.md)** - Durable persistence with querying (ADR 0025)

## Integration Order

Features are numbered in dependency order. When integrating into main:

**Implemented Features (01-08):**
1. Start with **01-channel-groupby** (prerequisite)
2. Continue with **02-message-core-refactor** (foundation)
3. Add **03-message-pubsub** (uses 01 and 02)
4. Add **04-message-router** (uses 02)
5. Add **05-message-cqrs** (uses 02 and 04)
6. Add **06-message-cloudevents** (uses 02)
7. Add **07-message-multiplex** (uses 03)
8. Add **08-middleware-package** (uses 04)

**Proposed Features (09-17) - Implementation Order:**
1. **16-core-pipe-refactoring** (PREREQUISITE - do first)
2. **09-cloudevents-mandatory** (depends on 16)
3. **10-non-generic-message** (depends on 09)
4. **11-contenttype-serialization** (depends on 10)
5. **12-internal-message-routing** (depends on 11)
6. **13-internal-message-loop** (depends on 12)
7. **14-nats-integration** (depends on 13)
8. **15-sql-event-store** (depends on 11)
9. **17-message-engine** (depends on 12, 13, 16)

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
- [ADR 0019](../adr/0019-cloudevents-mandatory.md) - CloudEvents Mandatory Specification
- [ADR 0020](../adr/0020-non-generic-message.md) - Non-Generic Message Type
- [ADR 0021](../adr/0021-contenttype-serialization.md) - ContentType-Based Serialization
- [ADR 0022](../adr/0022-internal-message-routing.md) - Internal Message Routing via Topic
- [ADR 0023](../adr/0023-internal-message-loop.md) - Internal Message Loop
- [ADR 0024](../adr/0024-destination-attribute.md) - Destination Attribute
- [ADR 0025](../adr/0025-sql-event-store.md) - SQL Event Store
- [ADR 0026](../adr/0026-pipe-processor-simplification.md) - Pipe and Processor Simplification
- [ADR 0027](../adr/0027-fan-out-pattern.md) - Fan-Out Pattern
- [ADR 0028](../adr/0028-generator-source-patterns.md) - Subscriber Patterns
- [ADR 0029](../adr/0029-message-engine.md) - Message Engine

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
