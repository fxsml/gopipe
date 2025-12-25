# ADR 0021: Marshaler Pattern

**Date:** 2025-12-22
**Status:** Proposed

## Context

Messages cross system boundaries as `[]byte`, but handlers work with typed Go data. We need bidirectional mapping between CloudEvents `type` attribute and Go types.

## Decision

Introduce a Marshaler with type registry:

```go
type Marshaler interface {
    Register(ceType string, prototype any)
    Name(v any) string
    TypeFor(ceType string) (reflect.Type, bool)
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
}
```

### Type Mapping

| Direction | Example |
|-----------|---------|
| Go → CE | `OrderCreated{}` → `"order.created"` |
| CE → Go | `"order.created"` → `OrderCreated{}` |

### Naming Strategy

For unregistered types, derive CE type from Go type name:
- `NamingKebab`: `OrderCreated` → `order.created`
- `NamingSimple`: `OrderCreated` → `OrderCreated`

## Consequences

**Benefits:**
- Handlers receive typed data, not `[]byte`
- Centralized type registry
- Auto-derive CE type from Go type

**Drawbacks:**
- Must register types for unmarshaling
- Reflection overhead

## Links

- Related: ADR 0018 (Interface Naming Conventions)
- Related: ADR 0019 (Remove Sender/Receiver)
- Related: ADR 0020 (Message Engine Architecture)
- Plan: [0002-marshaler.md](../plans/0002-marshaler.md)
