# Implementation Plans

This directory contains implementation plans for gopipe's evolution toward a CloudEvents-native messaging framework.

## Master Plan

**Start here:** [Architecture Roadmap](architecture-roadmap.md) - The big picture of where gopipe is going.

## Plan Hierarchy

```
architecture-roadmap.md (Master Plan)
├── layer-0-foundation-cleanup.md     (PRE-REQUISITE)
├── layer-1-message-standardization.md
├── layer-2-routing-infrastructure.md
├── layer-3-engine-orchestration.md
├── extension-event-persistence.md
└── cloudevents-standardization.md    (Original proposal)
```

## Layer Overview

| Layer | Title | Key Changes | Status |
|-------|-------|-------------|--------|
| 0 | [Foundation Cleanup](layer-0-foundation-cleanup.md) | ProcessorConfig, Subscriber, Middleware | PRE-REQUISITE |
| 1 | [Message Standardization](layer-1-message-standardization.md) | CloudEvents mandatory, non-generic Message | Proposed |
| 2 | [Routing Infrastructure](layer-2-routing-infrastructure.md) | Destination URIs, internal loops | Proposed |
| 3 | [Engine Orchestration](layer-3-engine-orchestration.md) | Engine, FanIn/FanOut | Proposed |
| Ext | [Event Persistence](extension-event-persistence.md) | SQL Event Store | Proposed |

## Implementation Order

```
Layer 0 (Foundation) ──────────────────────────────────────────────────────────┐
 - ProcessorConfig                                                             │
 - Middleware package                                                          │
 - Subscriber interface                                                        │
 - BroadcastConfig                                                             │
 - RoutingFanOut                                                               │
                                                                               │
Layer 1 (Messages) ◄───────────────────────────────────────────────────────────┤
 - CloudEvents validation                                                      │
 - Non-generic Message                                                         │
 - Type Registry                                                               │
 - ContentType serialization                                                   │
                                                                               │
Layer 2 (Routing) ◄────────────────────────────────────────────────────────────┤
 - Destination attribute                                                       │
 - InternalRouter                                                              │
 - MessageChannel                                                              │
 - InternalLoop                                                                │
                                                                               │
Layer 3 (Engine) ◄─────────────────────────────────────────────────────────────┘
 - RoutingFanIn
 - Engine orchestrator
 - Lifecycle management

Extension (Parallel after Layer 1) ────────────────────────────────────────────
 - EventStore interface
 - PostgreSQL/SQLite drivers
 - Transactional outbox
```

## Quick Reference

### Layer Dependencies

- **Layer 0** → No dependencies (start here)
- **Layer 1** → Layer 0
- **Layer 2** → Layer 1
- **Layer 3** → Layer 2
- **Extension** → Layer 1 (can be parallel with 2 & 3)

### Breaking Changes by Layer

| Layer | Breaking Changes |
|-------|------------------|
| 0 | `Option[In, Out]` → `ProcessorConfig`, `Generator` → `Subscriber` |
| 1 | `New()` returns error, `TypedMessage[T]` → `Message` |
| 2 | New `destination` attribute (additive) |
| 3 | None (Engine is additive) |
| Ext | None (new package) |

## Related Documentation

- [ADR Index](../adr/README.md) - Architecture Decision Records
- [Feature Index](../features/README.md) - Feature documentation
- [CLAUDE.md](../../CLAUDE.md) - Development procedures
