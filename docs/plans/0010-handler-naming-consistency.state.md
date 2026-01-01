# Plan 0010: Handler Naming Consistency

**Status:** Draft
**Created:** 2026-01-01

## Problem

Inconsistent naming between `Handler.NewInput()` and `TypeRegistry.NewInstance()`:

| Interface | Method | Signature | Purpose |
|-----------|--------|-----------|---------|
| `Handler` | `NewInput()` | `NewInput() any` | Create instance for THIS handler's type |
| `TypeRegistry` | `NewInstance()` | `NewInstance(ceType string) any` | Create instance for ANY registered type |

Both methods serve the same purpose: create a typed instance for unmarshaling. The naming difference (`NewInput` vs `NewInstance`) obscures this relationship.

## Current Implementation

```go
// handler.go
type Handler interface {
    EventType() string
    NewInput() any  // No parameter - type is intrinsic
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// registry.go
type TypeRegistry interface {
    NewInstance(ceType string) any  // Needs ceType for lookup
}

// router.go - bridges the two
func (r *Router) NewInstance(ceType string) any {
    entry, ok := r.handler(ceType)
    if !ok {
        return nil
    }
    return entry.handler.NewInput()  // Calls Handler.NewInput()
}
```

## Analysis

### Why signatures differ

The signatures differ for valid reasons:
- **Handler** already knows its type (via `EventType()`), so no lookup parameter needed
- **TypeRegistry** has many types, so it needs the ceType to find the correct factory

### Why names should match

Both create instances for unmarshaling. Using consistent naming clarifies this:
- Makes the relationship between Handler and TypeRegistry obvious
- Router's implementation becomes self-documenting: `entry.handler.NewInstance()`

## Proposed Change

Rename `Handler.NewInput()` to `Handler.NewInstance()`:

```go
// handler.go
type Handler interface {
    EventType() string
    NewInstance() any  // Renamed from NewInput
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

No signature change needed - the parameter difference is intentional and correct.

## Impact

### Files to modify:
1. `message/handler.go` - Interface and implementations
2. `message/router.go` - Call site in `NewInstance()`
3. `message/handler_test.go` - Test references

### Breaking change:
Yes - any external Handler implementations must rename `NewInput` to `NewInstance`.

## Implementation

```
[ ] Rename Handler.NewInput() to Handler.NewInstance() in interface
[ ] Update handler[T].NewInput() to NewInstance()
[ ] Update commandHandler[C,E].NewInput() to NewInstance()
[ ] Update Router.NewInstance() call site
[ ] Update tests
[ ] Run make test && make build && make vet
```

## Decision

**Recommendation:** Proceed with rename for API consistency before v1 release.
