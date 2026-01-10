# Design Patterns

This directory contains documentation for design patterns used in gopipe and goengine.

## Pattern Index

### Event-Driven Architecture

| Pattern | Description | Used In |
|---------|-------------|---------|
| [CQRS](cqrs.md) | Command Query Responsibility Segregation | goengine |

### Pipeline Patterns

| Pattern | Description | Used In |
|---------|-------------|---------|
| Middleware | Cross-cutting concerns | gopipe |
| Fan-Out/Fan-In | Parallel processing | gopipe |

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

## Related Documentation

- [ADRs](../adr/) - Architecture decisions
