# ADR 0021: Marshaler Pattern

**Date:** 2025-12-22
**Status:** Proposed (see [ADR 0022](0022-message-package-redesign.md))

## Context

Messages cross system boundaries as `[]byte`, but handlers work with typed Go data. We need bidirectional mapping between CloudEvents `type` attribute and Go types.

## Decision

Introduce a Marshaler with type registry and NamingStrategy:

```go
type Marshaler interface {
    Register(goType reflect.Type)              // register Go type, derive CE type via NamingStrategy
    TypeName(goType reflect.Type) string       // Go type → CE type (lookup or derive)
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
    ContentType() string
}
```

### Type Registry

Bidirectional mapping stored internally:

```
CE Type → Go Type    │    Go Type → CE Type
─────────────────    │    ─────────────────
"order.created" → OrderCreated
"order.shipped" → OrderShipped
```

- **Unmarshal**: CE type string → Go type → instantiate
- **TypeName**: Go type → CE type string

### NamingStrategy

NamingStrategy lives in Marshaler and derives CE type from Go type:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}
```

Implementations:
- `KebabNaming`: `OrderCreated` → `"order.created"`
- `SnakeNaming`: `OrderCreated` → `"order_created"`

### Auto-Registration

Types are auto-registered when `TypeName` is called for unregistered types. This enables the Engine to auto-register handler types without explicit registration.

## Consequences

**Benefits:**
- Handlers receive typed data, not `[]byte`
- Centralized type registry with bidirectional lookup
- Auto-derive CE type from Go type via NamingStrategy
- Auto-registration on TypeName simplifies usage

**Drawbacks:**
- Must have types registered for Unmarshal (explicit or auto)
- Reflection overhead

## Links

- Related: ADR 0018 (Interface Naming Conventions)
- Related: ADR 0019 (Remove Sender/Receiver)
- Related: ADR 0020 (Message Engine Architecture)
- Plan: [0002-marshaler.md](../plans/0002-marshaler.md)
