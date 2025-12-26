# ADR 0021: Codec and NamingStrategy

**Date:** 2025-12-22
**Updated:** 2025-12-27
**Status:** Proposed (see [ADR 0022](0022-message-package-redesign.md))

## Context

Messages cross system boundaries as `[]byte`, but handlers work with typed Go data. We need:
1. Serialization/deserialization
2. Convention for deriving CE type from Go type names

## Decision

Split into two separate components with clear responsibilities:

### Codec - Pure Serialization

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    ContentType() string  // e.g., "application/json"
}
```

Codec does one thing: convert between Go values and bytes. No type registry, no naming.

### NamingStrategy - Standalone Utility

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

NamingStrategy is a utility used by handler constructors to derive CE type from Go type.

### Handler.NewInput() - Instance Creation

No public TypeRegistry is needed. Handler provides `NewInput()` to create instances for unmarshaling:

```go
type Handler interface {
    EventType() string  // CE type - for routing
    NewInput() any      // creates new instance for unmarshaling
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

## Usage

### Handler Construction

```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"
// handler.NewInput() returns *OrderCreated for unmarshaling
```

### Engine Setup

```go
engine := message.NewEngine(message.EngineConfig{
    Codec: message.NewJSONCodec(),
})

// Engine routes by handler.EventType(), uses handler.NewInput() for unmarshaling
engine.AddHandler(handler, message.HandlerConfig{Name: "process-order"})
```

### Unmarshaling Flow

```go
// Engine receives []byte + CE type from input
// 1. Get handler for CE type, create instance
handler := engine.handlers[ceType]
instance := handler.NewInput()  // returns *OrderCreated

// 2. Unmarshal into instance
codec.Unmarshal(data, instance)
```

## Consequences

**Benefits:**
- Clean separation of concerns
- Codec is simple and testable
- No public TypeRegistry - Handler.NewInput() handles instance creation
- NamingStrategy encapsulated in handler constructors
- Handler is self-describing (knows its own CE type and how to create instances)

**Drawbacks:**
- Handler interface has 3 methods instead of 1
- Each handler must implement NewInput()

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
