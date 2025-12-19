# goengine Architecture Decision Records

ADRs documenting decisions for the goengine CloudEvents messaging engine.

**Note:** goengine will be a separate repository. ADRs are numbered independently starting at 0001.

## ADR Index

### Proposed (PRO-)

| ADR | Title | Dependencies |
|-----|-------|--------------|
| [PRO-0001](PRO-0001-saga-coordinator-pattern.md) | Saga Coordinator Pattern | - |
| [PRO-0002](PRO-0002-compensating-saga-pattern.md) | Compensating Saga Pattern | PRO-0001 |
| [PRO-0003](PRO-0003-transactional-outbox-pattern.md) | Transactional Outbox | - |
| [PRO-0004](PRO-0004-cloudevents-mandatory.md) | CloudEvents Mandatory | - |
| [PRO-0005](PRO-0005-non-generic-message.md) | Non-Generic Message | PRO-0004 |
| [PRO-0006](PRO-0006-contenttype-serialization.md) | ContentType Serialization | PRO-0005 |
| [PRO-0007](PRO-0007-internal-message-routing.md) | Internal Message Routing | PRO-0005 |
| [PRO-0008](PRO-0008-internal-message-loop.md) | Internal Message Loop | PRO-0007 |
| [PRO-0009](PRO-0009-destination-attribute.md) | Destination Attribute | PRO-0007 |
| [PRO-0010](PRO-0010-sql-event-store.md) | SQL Event Store | PRO-0003 |
| [PRO-0011](PRO-0011-message-engine.md) | Message Engine Architecture | PRO-0004-0009 |
| [PRO-0012](PRO-0012-google-uuid.md) | Google UUID for Message IDs | PRO-0004 |

## Status Prefixes

| Prefix | Meaning |
|--------|---------|
| `PRO-` | Proposed - Under consideration |
| `ACC-` | Accepted - Decision made |
| `IMP-` | Implemented - Fully implemented |
| `SUP-` | Superseded - Replaced by newer decision |

## Dependency Graph

```
CloudEvents Foundation
├── PRO-0004: CloudEvents Mandatory
│   ├── PRO-0012: Google UUID (external dependency)
│   └── PRO-0005: Non-Generic Message
│       ├── PRO-0006: ContentType Serialization
│       ├── PRO-0007: Internal Message Routing
│       │   ├── PRO-0008: Internal Message Loop
│       │   └── PRO-0009: Destination Attribute
│       └── PRO-0011: Message Engine (consolidates above)

Patterns
├── PRO-0001: Saga Coordinator
│   └── PRO-0002: Compensating Saga
└── PRO-0003: Transactional Outbox
    └── PRO-0010: SQL Event Store
```

## Related Documentation

- [goengine Plans](../plans/) - Implementation plans
- [Pattern Documentation](../../patterns/) - CQRS, Saga, Outbox patterns
