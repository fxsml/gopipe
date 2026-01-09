# Plan 0010: Handler Naming Consistency

**Status:** Implemented
**Created:** 2026-01-02

## Problem

Inconsistent naming between `Handler.NewInput()` and `TypeRegistry.NewInstance()`:

| Interface | Method | Signature | Purpose |
|-----------|--------|-----------|---------|
| `Handler` | `NewInput()` | `NewInput() any` | Create instance for THIS handler's type |
| `TypeRegistry` | `NewInstance()` | `NewInstance(ceType string) any` | Create instance for ANY registered type |

Both methods create typed instances for unmarshaling. Should they share the same signature?

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
    return entry.handler.NewInput()
}
```

## Options

### Option A: Rename only (different signatures)

Rename `Handler.NewInput()` to `Handler.NewInstance()` but keep signatures different:

```go
type Handler interface {
    EventType() string
    NewInstance() any  // Renamed, no parameter
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

**Pros:**
- Consistent naming
- Simple change
- Handler doesn't need to validate redundant parameter

**Cons:**
- Handler still cannot implement TypeRegistry directly
- Two methods with same name but different signatures

### Option B: Handler implements TypeRegistry

Add ceType parameter so Handler implements TypeRegistry:

```go
type Handler interface {
    EventType() string
    NewInstance(ceType string) any  // Same as TypeRegistry
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Handler now implements TypeRegistry
var _ TypeRegistry = (Handler)(nil)
```

**Pros:**
- Handler IS-A TypeRegistry (interface satisfaction)
- Could enable multi-type handlers
- Single consistent signature across the codebase

**Cons:**
- Parameter is redundant for single-type handlers (99% of cases)
- Handlers must either:
  - Ignore the parameter (confusing API)
  - Validate it matches EventType() (boilerplate)
  - Actually support multiple types (complexity)

### Option C: Keep current design

Keep `NewInput()` as-is. The names differ because the abstractions differ.

**Pros:**
- No breaking change
- Handler.NewInput() clearly means "for this handler"
- TypeRegistry.NewInstance(type) clearly means "lookup by type"

**Cons:**
- Naming inconsistency remains

## Multi-Type Handler Analysis

Could a handler process multiple CE types? Potential use cases:

1. **Versioned events**: `order.created.v1` and `order.created.v2`
2. **Related events**: `order.created` and `order.updated` with shared logic
3. **Generic handlers**: Logging handler that processes any message

### Critical Assessment

For multi-type handlers to work, we'd need:

```go
type MultiHandler interface {
    EventTypes() []string              // Multiple types
    NewInstance(ceType string) any     // Different instance per type
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

**Problems:**
1. `Handle()` receives `*Message` with already-unmarshaled `Data` - how does it know which type it got?
2. Registration becomes complex: register once for multiple types?
3. Current design handles this via multiple registrations of same handler logic
4. Generic handlers (logging) don't need typed unmarshaling - they use `*RawMessage`

**Conclusion:** Multi-type handlers add complexity without clear benefit. The current pattern of registering the same handler function multiple times (with different type parameters) achieves the same result more simply.

## Recommendation

**Option C: Keep current design.**

The naming difference reflects a real conceptual difference:
- `Handler.NewInput()` - "create input for ME" (self-referential)
- `TypeRegistry.NewInstance(type)` - "create instance for TYPE" (lookup)

Making Handler implement TypeRegistry would:
1. Add a redundant parameter to 99% of handlers
2. Encourage multi-type handlers which complicate the mental model
3. Blur the distinction between "a handler" and "a registry of handlers"

The current design is simple: one handler handles one type. Router composes handlers into a TypeRegistry. This composition is cleaner than forcing Handler to be a TypeRegistry.

### Option D: Rename TypeRegistry to InputRegistry

Keep Handler unchanged, rename the registry for consistency:

```go
// Handler unchanged
type Handler interface {
    EventType() string
    NewInput() any
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Renamed from TypeRegistry
type InputRegistry interface {
    NewInput(ceType string) any  // Renamed from NewInstance
}
```

**Pros:**
- Consistent naming: both use `NewInput`
- `InputRegistry` is more descriptive than `TypeRegistry`
- No change to Handler interface (less breaking)
- Signature difference preserved (intrinsic vs lookup)
- Clear purpose: registry creates inputs for unmarshaling

**Cons:**
- Breaking change for `TypeRegistry` users
- `FactoryMap` and `Router` need updated interface name

**Naming alternatives:**
- `InputRegistry` - concise, clear ✓
- `InputTypeRegistry` - verbose, "type" is redundant
- `UnmarshalRegistry` - too implementation-specific

## Decision

**Option D: Rename TypeRegistry to InputRegistry.**

This provides naming consistency (`NewInput` everywhere) while preserving the meaningful signature difference. The registry name becomes more descriptive of its purpose.

## Implementation

```
[x] Rename TypeRegistry to InputRegistry in registry.go
[x] Rename NewInstance to NewInput in InputRegistry interface
[x] Update FactoryMap to implement InputRegistry.NewInput
[x] Update Router to implement InputRegistry.NewInput
[x] Update pipes.go parameter type
[x] Update tests
[x] Run make test && make build && make vet
```

## Files to Modify

1. `message/registry.go` - Interface rename
2. `message/router.go` - Method rename, interface assertion
3. `message/pipes.go` - Parameter type
4. `message/engine.go` - Any references
5. `message/registry_test.go` - Test updates
6. `message/pipes_test.go` - Test updates
