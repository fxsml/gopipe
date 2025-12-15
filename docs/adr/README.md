# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for gopipe, documenting significant architectural decisions made during development.

## ADR Status

ADRs are categorized by their current status:

### Implemented ✅

These ADRs describe features that have been fully implemented and are part of the codebase:

- **[ADR 0001](0001-public-message-fields.md)** - Public Message Fields
- **[ADR 0002](0002-remove-properties-thread-safety.md)** - Remove Properties Thread Safety
- **[ADR 0003](0003-remove-noisy-properties.md)** - Remove Noisy Properties
- **[ADR 0004](0004-dual-message-types.md)** - Dual Message Types
- **[ADR 0005](0005-remove-functional-options.md)** - Remove Functional Options
- **[ADR 0006](0006-cqrs-implementation.md)** - CQRS Implementation
- **[ADR 0010](0010-pubsub-package-structure.md)** - Pub/Sub Package Structure
- **[ADR 0012](0012-multiplex-pubsub.md)** - Multiplex Pub/Sub
- **[ADR 0013](0013-processor-abstraction.md)** - Processor Abstraction
- **[ADR 0014](0014-composable-pipe.md)** - Composable Pipe
- **[ADR 0015](0015-middleware-pattern.md)** - Middleware Pattern
- **[ADR 0016](0016-channel-package.md)** - Channel Package
- **[ADR 0017](0017-message-acknowledgment.md)** - Message Acknowledgment

### Accepted ✓

These ADRs describe accepted decisions that guide current and future development:

- **[ADR 0018](0018-cloudevents-terminology.md)** - CloudEvents Terminology

### Proposed 💡

These ADRs describe proposed features that are documented but not yet implemented:

- **[ADR 0007](0007-saga-coordinator-pattern.md)** - Saga Coordinator Pattern
- **[ADR 0008](0008-compensating-saga-pattern.md)** - Compensating Saga Pattern
- **[ADR 0009](0009-transactional-outbox-pattern.md)** - Transactional Outbox Pattern
- **[ADR 0019](0019-cloudevents-mandatory.md)** - CloudEvents Mandatory Specification
- **[ADR 0020](0020-non-generic-message.md)** - Non-Generic Message Type
- **[ADR 0021](0021-contenttype-serialization.md)** - ContentType-Based Serialization
- **[ADR 0022](0022-internal-message-routing.md)** - Internal Message Routing via Topic
- **[ADR 0023](0023-internal-message-loop.md)** - Internal Message Loop
- **[ADR 0024](0024-destination-attribute.md)** - Destination Attribute
- **[ADR 0025](0025-sql-event-store.md)** - SQL Event Store
- **[ADR 0026](0026-pipe-processor-simplification.md)** - Pipe and Processor Simplification
- **[ADR 0027](0027-fan-out-pattern.md)** - Fan-Out Pattern
- **[ADR 0028](0028-generator-source-patterns.md)** - Subscriber Patterns
- **[ADR 0029](0029-message-engine.md)** - Message Engine

### Superseded ⛔

These ADRs have been superseded by later decisions:

- **[ADR 0011](0011-cloudevents-compatibility.md)** - CloudEvents Compatibility (superseded by ADR 0018)

## ADR Format

Each ADR follows a standard format:

```markdown
# ADR NNNN: Title

**Date:** YYYY-MM-DD
**Status:** [Proposed|Accepted|Implemented|Superseded]

## Context
What is the issue we're facing?

## Decision
What decision have we made?

## Consequences
What are the positive and negative consequences?

## Links (optional)
Related ADRs, documentation, or code
```

## Feature Mapping

ADRs are organized into logical feature groups. See [../features/](../features/) for complete feature documentation.

### Core Message Refactoring (ADRs 0001-0005)
Foundation for simplified message handling:
- ADR 0001: Public Message Fields
- ADR 0002: Remove Properties Thread Safety
- ADR 0003: Remove Noisy Properties
- ADR 0004: Dual Message Types
- ADR 0005: Remove Functional Options

Related: [Feature 02-message-core-refactor](../features/02-message-core-refactor.md)

### CQRS and Saga Patterns (ADRs 0006-0009)
Event-driven architecture support:
- ADR 0006: CQRS Implementation ✅
- ADR 0007: Saga Coordinator Pattern 💡
- ADR 0008: Compensating Saga Pattern 💡
- ADR 0009: Transactional Outbox Pattern 💡

Related: [Feature 05-message-cqrs](../features/05-message-cqrs.md)

### Pub/Sub Infrastructure (ADRs 0010-0012)
Message broker and routing:
- ADR 0010: Pub/Sub Package Structure
- ADR 0012: Multiplex Pub/Sub

Related:
- [Feature 03-message-pubsub](../features/03-message-pubsub.md)
- [Feature 07-message-multiplex](../features/07-message-multiplex.md)

### Pipeline Abstractions (ADRs 0013-0015)
Composable message processing:
- ADR 0013: Processor Abstraction
- ADR 0014: Composable Pipe
- ADR 0015: Middleware Pattern

Related:
- [Feature 04-message-router](../features/04-message-router.md)
- [Feature 08-middleware-package](../features/08-middleware-package.md)

### Infrastructure (ADRs 0016-0017)
Supporting utilities:
- ADR 0016: Channel Package
- ADR 0017: Message Acknowledgment

Related:
- [Feature 01-channel-groupby](../features/01-channel-groupby.md)
- [Feature 02-message-core-refactor](../features/02-message-core-refactor.md)

### Standards and Compatibility (ADR 0018)
Industry standard alignment:
- ADR 0018: CloudEvents Terminology

Related: [Feature 06-message-cloudevents](../features/06-message-cloudevents.md)

### Core Pipe Refactoring (ADRs 0026-0029) 💡
Proposed prerequisite for CloudEvents standardization:
- ADR 0026: Pipe and Processor Simplification 💡
- ADR 0027: Fan-Out Pattern 💡
- ADR 0028: Subscriber Patterns 💡
- ADR 0029: Message Engine 💡

Related:
- [Feature 16-core-pipe-refactoring](../features/16-core-pipe-refactoring.md)
- [Feature 17-message-engine](../features/17-message-engine.md)

### CloudEvents Standardization (ADRs 0019-0025) 💡
Proposed: Making CloudEvents mandatory and enabling composable internal pipelines:
- ADR 0019: CloudEvents Mandatory Specification 💡
- ADR 0020: Non-Generic Message Type 💡
- ADR 0021: ContentType-Based Serialization 💡
- ADR 0022: Internal Message Routing via Topic 💡
- ADR 0023: Internal Message Loop 💡
- ADR 0024: Destination Attribute 💡
- ADR 0025: SQL Event Store 💡

Related:
- [Feature 09-cloudevents-mandatory](../features/09-cloudevents-mandatory.md)
- [Feature 10-non-generic-message](../features/10-non-generic-message.md)
- [Feature 11-contenttype-serialization](../features/11-contenttype-serialization.md)
- [Feature 12-internal-message-routing](../features/12-internal-message-routing.md)
- [Feature 13-internal-message-loop](../features/13-internal-message-loop.md)
- [Feature 14-nats-integration](../features/14-nats-integration.md)
- [Feature 15-sql-event-store](../features/15-sql-event-store.md)
- [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md)

## Reading Order

For understanding the complete architecture, read ADRs in this order:

1. **Core Message Design** (0001-0005) - Foundation
2. **Package Structure** (0010) - Organization
3. **Infrastructure** (0016-0017) - Supporting components
4. **Routing & Handling** (0013-0015) - Message processing
5. **Pub/Sub** (0012) - Message distribution
6. **Standards** (0018) - CloudEvents alignment
7. **CQRS** (0006) - Event-driven patterns
8. **Advanced Patterns** (0007-0009) - Sagas and outbox (proposed)
9. **Core Pipe Refactoring** (0026-0029) - Proposed simplification (prerequisite)
10. **CloudEvents Standardization** (0019-0025) - Proposed mandatory CE, internal routing, message loop, and event store

## Timeline

- **2025-11-01**: Initial message refactoring (ADRs 0001-0005)
- **2025-12-08**: CQRS and pub/sub implementation (ADRs 0006-0017)
- **2025-12-11**: CloudEvents terminology standardization (ADR 0018)
- **2025-12-13**: CloudEvents standardization plan proposed (ADRs 0019-0025)
- **2025-12-13**: Core pipe refactoring plan proposed (ADRs 0026-0028)
- **2025-12-15**: Message Engine proposed (ADR 0029)

## Related Documentation

- [Features](../features/) - Feature-level documentation for each implementation
- [Analysis Documents](../) - Detailed design analysis and comparisons
- [Examples](../../examples/) - Working code examples
