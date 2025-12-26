# ADR 0021: Codec, TypeRegistry, and NamingStrategy

**Date:** 2025-12-22
**Updated:** 2025-12-26
**Status:** Proposed (see [ADR 0022](0022-message-package-redesign.md))

## Context

Messages cross system boundaries as `[]byte`, but handlers work with typed Go data. We need:
1. Serialization/deserialization
2. Mapping between CloudEvents `type` attribute and Go types
3. Convention for deriving CE type from Go type names

## Decision

Split into three separate components with clear responsibilities:

### Codec - Pure Serialization

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    ContentType() string  // e.g., "application/json"
}
```

Codec does one thing: convert between Go values and bytes. No type registry, no naming.

### TypeRegistry - CE Type ↔ Go Type Mapping

```go
type TypeRegistry interface {
    Register(ceType string, goType reflect.Type)
    Lookup(ceType string) (reflect.Type, bool)
}
```

TypeRegistry maps CE types to Go types for unmarshaling. Registration is explicit.

### NamingStrategy - Standalone Utility

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

NamingStrategy is a utility used by handler constructors to derive CE type from Go type.

## Usage

### Handler Construction

```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"
// handler.GoType() returns reflect.Type of OrderCreated
```

### Engine Setup

```go
engine := message.NewEngine(message.EngineConfig{
    Codec:    message.NewJSONCodec(),
    Registry: message.NewTypeRegistry(),
})

// Handler self-registers via EventType() and GoType()
engine.AddHandler(handler, message.HandlerConfig{Name: "process-order"})
```

### Unmarshaling Flow

```go
// Engine receives []byte + CE type from input
// 1. Lookup Go type in registry
goType, _ := registry.Lookup(ceType)

// 2. Instantiate and unmarshal
instance := reflect.New(goType).Interface()
codec.Unmarshal(data, instance)
```

## Consequences

**Benefits:**
- Clean separation of concerns
- Codec is simple and testable
- TypeRegistry has explicit registration (no magic)
- NamingStrategy encapsulated in handler constructors
- Handler is self-describing (knows its own CE type)

**Drawbacks:**
- Must register types in TypeRegistry (via AddHandler)
- Reflection overhead for instantiation

## Rejected: Combined Marshaler

```go
type Marshaler interface {
    Register(goType reflect.Type)        // ❌
    TypeName(goType reflect.Type) string // ❌
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
}
```

**Why rejected:** Marshaler was doing too much - serialization, type registry, and naming all in one interface. Splitting into three focused components is cleaner and more idiomatic.

## Links

- Related: ADR 0018 (Interface Naming Conventions)
- Related: ADR 0019 (Remove Sender/Receiver)
- Related: ADR 0020 (Message Engine Architecture)
- Plan: [0002-marshaler.md](../plans/0002-marshaler.md)
