# Design Patterns

This directory contains documentation for design patterns used in gopipe and goengine.

## Pattern Index

### Event-Driven Architecture

| Pattern | Description | Used In |
|---------|-------------|---------|
| [CQRS](cqrs.md) | Command Query Responsibility Segregation | goengine |
| [Saga](saga.md) | Distributed transaction coordination | goengine |
| [Outbox](outbox.md) | Reliable event publishing | goengine |

### Pipeline Patterns

| Pattern | Description | Used In |
|---------|-------------|---------|
| [Middleware](middleware.md) | Cross-cutting concerns | gopipe |
| [Fan-Out/Fan-In](fan-patterns.md) | Parallel processing | gopipe |

## Quick Reference

### CQRS (Command Query Responsibility Segregation)

Separates read and write operations into different models.

```
Command → CommandHandler → Event → EventStore
                                       ↓
Query ← QueryHandler ← ReadModel ← Projection
```

**When to use:**
- Complex domain logic
- Different read/write scaling needs
- Event sourcing requirements

### Saga Pattern

Coordinates distributed transactions across services.

```
Start → Step1 → Step2 → Step3 → Complete
          ↓       ↓       ↓
       Compensate if any step fails
```

**When to use:**
- Multi-service transactions
- Long-running workflows
- Eventual consistency acceptable

### Outbox Pattern

Ensures reliable event publishing with database transactions.

```
Transaction {
    1. Update database
    2. Write to outbox table
}
Background:
    3. Poll outbox → Publish to broker
    4. Mark as published
```

**When to use:**
- At-least-once delivery required
- Database + broker consistency needed
- Cannot use distributed transactions

## Related Documentation

- [goengine Plans](../goengine/plans/) - Pattern implementations
- [goengine ADRs](../goengine/adr/) - Architecture decisions
