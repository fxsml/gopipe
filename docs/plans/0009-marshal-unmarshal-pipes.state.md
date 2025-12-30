# State: Marshal/Unmarshal Public Pipe Components

**Status:** Revised - Simplified Design
**Related:** Plan 0008 (Release Review)

---

## Goal

Expose marshal/unmarshal as standalone public pipes that can be used independently of Engine.

---

## Simplified Design

### TypeRegistry Interface (1 method)

```go
// TypeRegistry creates typed instances for unmarshaling.
type TypeRegistry interface {
    NewInstance(ceType string) any  // nil if unknown type
}
```

That's it. One method. The consumer (UnmarshalPipe) only needs to create instances.

### Router Implements TypeRegistry

```go
// Router implements TypeRegistry using existing handler.NewInput().
func (r *Router) NewInstance(ceType string) any {
    entry, ok := r.handler(ceType)
    if !ok {
        return nil
    }
    return entry.handler.NewInput()
}

var _ TypeRegistry = (*Router)(nil)
```

### Standalone Registry (simple map)

```go
// FactoryMap is a simple TypeRegistry for standalone use.
type FactoryMap map[string]func() any

func (m FactoryMap) NewInstance(ceType string) any {
    if f, ok := m[ceType]; ok {
        return f()
    }
    return nil
}

var _ TypeRegistry = (FactoryMap)(nil)
```

Usage:
```go
registry := message.FactoryMap{
    "order.command": func() any { return new(OrderCommand) },
    "user.command":  func() any { return new(UserCommand) },
}
```

---

## Public Pipes

### UnmarshalPipe

```go
// NewUnmarshalPipe creates a pipe that unmarshals RawMessage to Message.
func NewUnmarshalPipe(registry TypeRegistry, marshaler Marshaler, cfg pipe.Config) *pipe.ProcessPipe[*RawMessage, *Message]
```

Internally:
```go
func unmarshal(ctx context.Context, raw *RawMessage) (*Message, error) {
    ceType, _ := raw.Attributes["type"].(string)
    instance := registry.NewInstance(ceType)
    if instance == nil {
        return nil, fmt.Errorf("unknown type: %s", ceType)
    }
    if err := marshaler.Unmarshal(raw.Data, instance); err != nil {
        return nil, err
    }
    return &Message{Data: instance, Attributes: raw.Attributes, acking: raw.acking}, nil
}
```

### MarshalPipe

```go
// NewMarshalPipe creates a pipe that marshals Message to RawMessage.
// No registry needed - just marshals msg.Data.
func NewMarshalPipe(marshaler Marshaler, cfg pipe.Config) *pipe.ProcessPipe[*Message, *RawMessage]
```

Internally:
```go
func marshal(ctx context.Context, msg *Message) (*RawMessage, error) {
    data, err := marshaler.Marshal(msg.Data)
    if err != nil {
        return nil, err
    }
    return &RawMessage{Data: data, Attributes: msg.Attributes, acking: msg.acking}, nil
}
```

---

## What NOT to Add

| Concept | Why Not Needed |
|---------|----------------|
| `TypeEntry` interface | Lookup already has CE type as key |
| `Register` in interface | Implementation detail |
| `RegistryHandler` | Premature abstraction |
| `HandlerFunc` | No use case yet |
| `TypeOf[T]` helper | Nice but not essential |
| `NewInstance()` on handlers | Use existing `NewInput()` |

---

## Changes to Current Implementation

### Remove (overengineered)

From `registry.go`:
- `TypeEntry` interface
- `TypeRegistry` with Lookup/Register (replace with simpler version)
- `RegistryHandler` interface
- `HandlerFunc` type
- `TypeOf[T]` helper
- `MapRegistry` type (replace with `FactoryMap`)
- `typeEntry` struct
- `factoryEntry` struct

From `handler.go`:
- `NewInstance()` methods (duplicate of `NewInput()`)
- TypeEntry compliance checks

From `router.go`:
- `Lookup()` method
- `Register()` method

From `errors.go`:
- `ErrNotAHandler`

### Keep/Add

In `registry.go`:
```go
// TypeRegistry creates typed instances for unmarshaling.
type TypeRegistry interface {
    NewInstance(ceType string) any
}

// FactoryMap is a simple TypeRegistry for standalone use.
type FactoryMap map[string]func() any

func (m FactoryMap) NewInstance(ceType string) any {
    if f, ok := m[ceType]; ok {
        return f()
    }
    return nil
}

var _ TypeRegistry = (FactoryMap)(nil)
```

In `router.go`:
```go
func (r *Router) NewInstance(ceType string) any {
    entry, ok := r.handler(ceType)
    if !ok {
        return nil
    }
    return entry.handler.NewInput()
}

var _ TypeRegistry = (*Router)(nil)
```

---

## Usage Examples

### Standalone Pipeline

```go
registry := message.FactoryMap{
    "order.created": func() any { return new(OrderCreated) },
}

unmarshal := message.NewUnmarshalPipe(registry, marshaler, pipe.Config{
    Concurrency: 4,
})
marshal := message.NewMarshalPipe(marshaler, pipe.Config{})

// Raw JSON in → typed messages → process → JSON out
rawIn := make(chan *message.RawMessage)
typed, _ := unmarshal.Pipe(ctx, rawIn)
processed := process(typed)
rawOut, _ := marshal.Pipe(ctx, processed)
```

### With Engine (Router as registry)

```go
engine := message.NewEngine(cfg)
engine.AddHandler(orderHandler, message.HandlerConfig{})

// Router implements TypeRegistry
unmarshal := message.NewUnmarshalPipe(engine.Router(), marshaler, pipe.Config{})
```

---

## Summary

| Aspect | Before (overengineered) | After (simple) |
|--------|------------------------|----------------|
| Interface methods | 2 (Lookup + Register) | 1 (NewInstance) |
| New types | 6 | 2 |
| Lines of code | ~110 | ~25 |
| Handler changes | NewInstance() added | None |
| New errors | 1 | 0 |

**Principle:** Only add what's needed for the immediate use case.
