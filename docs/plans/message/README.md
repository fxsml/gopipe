# goengine Implementation Plans

Plans for the goengine CloudEvents messaging engine.

**Note:** goengine will be a **separate repository** (`fxsml/goengine`).

## Plan Index

### Repository Setup (Start Here)

| Plan | Title | Priority |
|------|-------|----------|
| [PRO-0001](PRO-0001-repo-init/) | Repository Initialization | High |
| [PRO-0002](PRO-0002-migration/) | Migration from gopipe | High |

### Message Layer

| Plan | Title | Status |
|------|-------|--------|
| [PRO-0003](PRO-0003-message-standardization/) | Message Standardization | Proposed |
| [PRO-0011](PRO-0011-uuid-integration/) | UUID Integration (google/uuid) | Proposed |

### Routing Layer

| Plan | Title | Status |
|------|-------|--------|
| [PRO-0004](PRO-0004-routing-infrastructure/) | Routing Infrastructure | Proposed |

### Engine Layer

| Plan | Title | Status |
|------|-------|--------|
| [PRO-0005](PRO-0005-engine-orchestration/) | Engine Orchestration | Proposed |

### Extensions

| Plan | Title | Status |
|------|-------|--------|
| [PRO-0006](PRO-0006-event-persistence/) | Event Persistence | Proposed |
| [PRO-0007](PRO-0007-broker-adapters/) | Broker Adapters | Proposed |
| [PRO-0008](PRO-0008-cdc-subscriber/) | CDC Subscriber | Proposed |
| [PRO-0010](PRO-0010-broker-concerns/) | Broker Concerns (Ack, Lifecycle, Testing) | Proposed |

### Reference

| Plan | Title | Notes |
|------|-------|-------|
| [PRO-0009](PRO-0009-cloudevents-overview/) | CloudEvents Overview | Original proposal |

## Implementation Order

```
Phase 1: Setup
├── PRO-0001: Repository Init
└── PRO-0002: Migration from gopipe

Phase 2: Core
├── PRO-0003: Message Standardization
├── PRO-0011: UUID Integration
└── PRO-0004: Routing Infrastructure

Phase 3: Orchestration
└── PRO-0005: Engine Orchestration

Phase 4: Extensions
├── PRO-0006: Event Persistence
├── PRO-0007: Broker Adapters
├── PRO-0008: CDC Subscriber
└── PRO-0010: Broker Concerns
```

## Dependencies on gopipe

goengine depends on gopipe core cleanup:

| gopipe Plan | goengine Dependency |
|-------------|---------------------|
| PRO-0001: ProcessorConfig | Required for handlers |
| PRO-0003: Subscriber Interface | Required for subscribers |
| PRO-0006: Package Restructuring | Clean interface boundary |

## Related Documentation

- [goengine ADRs](../adr/) - Architecture decisions
- [gopipe Plans](../../plans/) - Core library plans
- [Patterns](../../patterns/) - CQRS, Saga, Outbox
