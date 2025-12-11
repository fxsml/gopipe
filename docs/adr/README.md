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

## Timeline

- **2025-11-01**: Initial message refactoring (ADRs 0001-0005)
- **2025-12-08**: CQRS and pub/sub implementation (ADRs 0006-0017)
- **2025-12-11**: CloudEvents terminology standardization (ADR 0018)

## Related Documentation

- [Features](../features/) - Feature-level documentation for each implementation
- [Analysis Documents](../) - Detailed design analysis and comparisons
- [Examples](../../examples/) - Working code examples
